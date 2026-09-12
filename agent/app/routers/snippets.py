from datetime import datetime, timezone
from typing import Optional
from fastapi import APIRouter, HTTPException, Query, Response, status
from bson import ObjectId
import pymongo
from app.config import settings
from app.schemas import (
    SnippetIngestRequest,
    SnippetIngestResponse,
    SnippetListResponse,
    SnippetItem,
    SnippetDetailResponse,
    SnippetPatchRequest,
    SnippetSearchRequest,
    SnippetSearchResponse,
    SearchResultItem,
)
from app.vectorstore import get_mongo_client, get_vectorstore, retriever

router = APIRouter()


def get_snippets_collection():
    client = get_mongo_client()
    return client[settings.MONGODB_DB]["manim_snippets"]


@router.post("/snippets/ingest", response_model=SnippetIngestResponse, status_code=status.HTTP_201_CREATED)
async def ingest_snippet(req: SnippetIngestRequest):
    """POST /snippets/ingest - Ingest a snippet into Atlas vector store."""
    try:
        col = get_snippets_collection()
        existing = col.find_one({"source": req.source})
        if existing:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail={
                    "error": {
                        "code": "duplicate_snippet",
                        "message": "Snippet with identical source code already exists.",
                        "details": {"id": str(existing["_id"])},
                    }
                },
            )

        store = get_vectorstore()
        doc_content = f"{req.title}\n{req.description}\n" + " ".join(req.tags)
        doc_metadata = {
            "title": req.title,
            "description": req.description,
            "category": req.category,
            "tags": req.tags,
            "source": req.source,
            "origin": req.origin,
            "verified": req.verified,
            "created_at": datetime.now(timezone.utc).isoformat(),
        }

        ids = store.add_texts(texts=[doc_content], metadatas=[doc_metadata])
        snippet_id = ids[0] if ids else str(ObjectId())

        return SnippetIngestResponse(id=snippet_id, embedded=True)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={"error": {"code": "internal", "message": str(e)}},
        )


@router.get("/snippets", response_model=SnippetListResponse)
async def list_snippets(
    verified: Optional[bool] = None,
    origin: Optional[str] = None,
    limit: int = Query(50, ge=1, le=100),
    cursor: Optional[str] = None,
):
    """GET /snippets - List snippets with optional filters."""
    try:
        col = get_snippets_collection()
        query = {}
        if verified is not None:
            query["verified"] = verified
        if origin is not None:
            query["origin"] = origin
        if cursor:
            query["_id"] = {"$gt": ObjectId(cursor)}

        cursor_obj = col.find(query).limit(limit)
        items = []
        next_cursor = None
        for doc in cursor_obj:
            doc_id = str(doc["_id"])
            items.append(
                SnippetItem(
                    id=doc_id,
                    title=doc.get("title", ""),
                    category=doc.get("category", "general"),
                    origin=doc.get("origin", "seed"),
                    verified=doc.get("verified", False),
                    created_at=doc.get("created_at", datetime.now(timezone.utc).isoformat()),
                )
            )
            next_cursor = doc_id

        return SnippetListResponse(snippets=items, next_cursor=next_cursor if len(items) == limit else None)
    except Exception as e:
        return SnippetListResponse(snippets=[], next_cursor=None)


@router.get("/snippets/{snippet_id}", response_model=SnippetDetailResponse)
async def get_snippet(snippet_id: str):
    """GET /snippets/{id} - Fetch single snippet."""
    try:
        col = get_snippets_collection()
        doc = col.find_one({"_id": ObjectId(snippet_id)})
        if not doc:
            raise HTTPException(status_code=404, detail={"error": {"code": "snippet_not_found", "message": "Snippet not found"}})
        return SnippetDetailResponse(
            id=str(doc["_id"]),
            title=doc.get("title", ""),
            description=doc.get("description", ""),
            category=doc.get("category", ""),
            tags=doc.get("tags", []),
            source=doc.get("source", ""),
            origin=doc.get("origin", ""),
            verified=doc.get("verified", False),
            created_at=doc.get("created_at", ""),
        )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=404, detail={"error": {"code": "snippet_not_found", "message": str(e)}})


@router.patch("/snippets/{snippet_id}", response_model=SnippetDetailResponse)
async def patch_snippet(snippet_id: str, req: SnippetPatchRequest):
    """PATCH /snippets/{id} - Promote, demote, or edit snippet."""
    try:
        col = get_snippets_collection()
        update_fields = {k: v for k, v in req.model_dump().items() if v is not None}
        if not update_fields:
            return await get_snippet(snippet_id)

        result = col.find_one_and_update(
            {"_id": ObjectId(snippet_id)},
            {"$set": update_fields},
            return_document=pymongo.ReturnDocument.AFTER,
        )
        if not result:
            raise HTTPException(status_code=404, detail={"error": {"code": "snippet_not_found", "message": "Snippet not found"}})

        return SnippetDetailResponse(
            id=str(result["_id"]),
            title=result.get("title", ""),
            description=result.get("description", ""),
            category=result.get("category", ""),
            tags=result.get("tags", []),
            source=result.get("source", ""),
            origin=result.get("origin", ""),
            verified=result.get("verified", False),
            created_at=result.get("created_at", ""),
        )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail={"error": {"code": "internal", "message": str(e)}})


@router.delete("/snippets/{snippet_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_snippet(snippet_id: str):
    """DELETE /snippets/{id} - Delete snippet."""
    try:
        col = get_snippets_collection()
        res = col.delete_one({"_id": ObjectId(snippet_id)})
        if res.deleted_count == 0:
            raise HTTPException(status_code=404, detail={"error": {"code": "snippet_not_found", "message": "Snippet not found"}})
        return Response(status_code=status.HTTP_204_NO_CONTENT)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail={"error": {"code": "internal", "message": str(e)}})


@router.post("/snippets/search", response_model=SnippetSearchResponse)
async def search_snippets(req: SnippetSearchRequest):
    """POST /snippets/search - Debug route to test snippet retrieval."""
    try:
        query_text = f"{req.scene.narration} {req.scene.visual} {req.hint}".strip()
        r = retriever(req.category, k=req.k)
        docs = r.invoke(query_text)
        results = []
        for d in docs:
            results.append(
                SearchResultItem(
                    id=str(d.metadata.get("_id", "seed")),
                    title=d.metadata.get("title", ""),
                    description=d.metadata.get("description", ""),
                    source=d.metadata.get("source", ""),
                    score=float(d.metadata.get("score", 0.9)),
                )
            )
        return SnippetSearchResponse(snippets=results)
    except Exception as e:
        return SnippetSearchResponse(snippets=[])
