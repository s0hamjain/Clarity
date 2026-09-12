import logging
from typing import Optional
import pymongo
from langchain_mongodb import MongoDBAtlasVectorSearch
from langchain_voyageai import VoyageAIEmbeddings
from app.config import settings

logger = logging.getLogger(__name__)

_mongo_client: Optional[pymongo.MongoClient] = None
_vectorstore_instance: Optional[MongoDBAtlasVectorSearch] = None


def get_mongo_client() -> pymongo.MongoClient:
    global _mongo_client
    if _mongo_client is None:
        _mongo_client = pymongo.MongoClient(
            settings.MONGODB_URI,
            serverSelectionTimeoutMS=2000,
        )
    return _mongo_client


def get_vectorstore() -> MongoDBAtlasVectorSearch:
    global _vectorstore_instance
    if _vectorstore_instance is None:
        client = get_mongo_client()
        db = client[settings.MONGODB_DB]
        collection = db["manim_snippets"]
        embeddings = VoyageAIEmbeddings(
            model=settings.EMBED_MODEL,
            voyage_api_key=settings.VOYAGE_API_KEY,
        )
        _vectorstore_instance = MongoDBAtlasVectorSearch(
            collection=collection,
            embedding=embeddings,
            index_name="snippets_vector",
            text_key="page_content",
            embedding_key="embedding",
        )
    return _vectorstore_instance


def retriever(category: str, k: int = 3):
    """Factory returning a VectorStoreRetriever filtering on verified=True and matching category."""
    store = get_vectorstore()
    pre_filter = {
        "verified": True,
        "category": {"$in": [category, "general"]},
    }
    return store.as_retriever(
        search_kwargs={
            "k": k,
            "pre_filter": pre_filter,
        }
    )
