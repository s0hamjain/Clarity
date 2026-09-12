import argparse
from datetime import datetime, timezone
import re
import sys
from pathlib import Path

# Add agent root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import pymongo
from app.config import settings
from app.vectorstore import get_mongo_client, get_vectorstore
from langchain_core.documents import Document

SAMPLES_DIR = Path(__file__).resolve().parent.parent.parent / "samples"


def slugify(text: str) -> str:
    """Turn text into a URL/ID slug."""
    text = text.lower().strip()
    text = re.sub(r"[^\w\s-]", "", text)
    return re.sub(r"[\s_-]+", "_", text)


def parse_docstring_header(source: str):
    """Parse docstring header formatted with title:, description:, category:, tags:."""
    title = "Untitled Snippet"
    description = ""
    category = "general"
    tags = []

    match = re.search(r'"""(.*?)"""', source, re.DOTALL)
    if match:
        header = match.group(1)
        for line in header.splitlines():
            line = line.strip()
            if line.startswith("title:"):
                title = line.split(":", 1)[1].strip()
            elif line.startswith("description:"):
                description = line.split(":", 1)[1].strip()
            elif line.startswith("category:"):
                category = line.split(":", 1)[1].strip()
            elif line.startswith("tags:"):
                raw_tags = line.split(":", 1)[1].strip()
                tags = [t.strip() for t in raw_tags.split(",") if t.strip()]

    return title, description, category, tags


def create_vector_search_index():
    """Create the Atlas Vector Search index 'snippets_vector' on manim_snippets."""
    print("Connecting to MongoDB Atlas to create Vector Search index 'snippets_vector'...")
    client = get_mongo_client()
    db = client[settings.MONGODB_DB]
    collection = db["manim_snippets"]

    index_definition = {
        "name": "snippets_vector",
        "type": "vectorSearch",
        "definition": {
            "fields": [
                {
                    "type": "vector",
                    "path": "embedding",
                    "numDimensions": 1024,
                    "similarity": "cosine",
                },
                {"type": "filter", "path": "verified"},
                {"type": "filter", "path": "category"},
            ]
        },
    }

    try:
        collection.create_search_index(model=index_definition)
        print("Successfully requested creation of 'snippets_vector' index in Atlas.")
    except Exception as e:
        print(f"Index creation request failed or index already exists: {e}")


def seed_snippets():
    """Read samples/*.py files, embed, and upsert documents into Atlas."""
    if not SAMPLES_DIR.exists():
        print(f"Directory {SAMPLES_DIR} does not exist yet. No seed samples to process.")
        return

    sample_files = list(SAMPLES_DIR.glob("*.py"))
    if not sample_files:
        print(f"No .py files found in {SAMPLES_DIR}.")
        return

    store = get_vectorstore()
    docs = []
    ids = []

    for file_path in sample_files:
        source = file_path.read_text(encoding="utf-8")
        title, description, category, tags = parse_docstring_header(source)
        page_content = f"{title}\n{description}\n" + " ".join(tags)

        doc = Document(
            page_content=page_content,
            metadata={
                "title": title,
                "description": description,
                "category": category,
                "tags": tags,
                "source": source,
                "origin": "seed",
                "verified": True,
                "created_at": datetime.now(timezone.utc).isoformat(),
            },
        )
        docs.append(doc)
        ids.append(slugify(title))

    if docs:
        print(f"Upserting {len(docs)} seed snippets into MongoDB Atlas vector store...")
        store.add_documents(documents=docs, ids=ids)
        print("Successfully seeded snippets.")


def main():
    parser = argparse.ArgumentParser(description="Seed snippet corpus into MongoDB Atlas Vector Search")
    parser.add_argument("--create-index", action="store_true", help="Create the snippets_vector Atlas index")
    args = parser.parse_args()

    if args.create_index:
        create_vector_search_index()

    seed_snippets()


if __name__ == "__main__":
    main()
