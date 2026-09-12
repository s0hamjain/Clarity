import argparse
from datetime import datetime, timezone
import re
import sys
from pathlib import Path

# Add agent root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app.vectorstore import get_vectorstore
from langchain_core.documents import Document

DOCS_DIR = Path(__file__).resolve().parent.parent / "manim_docs"


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


def seed_manim_docs():
    """Read manim_docs/*.py files (verbatim Manim CE example gallery entries),
    embed, and upsert them into Atlas with origin='manim_docs'."""
    if not DOCS_DIR.exists():
        print(f"Directory {DOCS_DIR} does not exist yet. No manim_docs to process.")
        return

    doc_files = sorted(DOCS_DIR.glob("*.py"))
    if not doc_files:
        print(f"No .py files found in {DOCS_DIR}.")
        return

    store = get_vectorstore()
    docs = []
    ids = []

    for file_path in doc_files:
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
                "origin": "manim_docs",
                "verified": True,
                "created_at": datetime.now(timezone.utc).isoformat(),
            },
        )
        docs.append(doc)
        ids.append(f"manim_docs_{slugify(title)}")

    if docs:
        print(f"Upserting {len(docs)} manim_docs snippets into MongoDB Atlas vector store...")
        store.add_documents(documents=docs, ids=ids)
        print("Successfully seeded manim_docs snippets.")


def main():
    parser = argparse.ArgumentParser(description="Seed Manim CE documentation examples into MongoDB Atlas Vector Search")
    seed_manim_docs()


if __name__ == "__main__":
    main()
