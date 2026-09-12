from pathlib import Path
from functools import lru_cache
from langchain_core.prompts import ChatPromptTemplate, SystemMessagePromptTemplate, HumanMessagePromptTemplate

PROMPTS_DIR = Path(__file__).parent / "prompts"


@lru_cache(maxsize=32)
def load_prompt(name: str) -> ChatPromptTemplate:
    """Load a markdown prompt template from app/prompts/{name}.md and return a ChatPromptTemplate."""
    file_path = PROMPTS_DIR / f"{name}.md"
    if not file_path.exists():
        raise FileNotFoundError(f"Prompt file not found: {file_path}")

    content = file_path.read_text(encoding="utf-8").strip()

    if "---" in content:
        parts = content.split("---", 1)
        system_text = parts[0].strip()
        human_text = parts[1].strip()
        return ChatPromptTemplate.from_messages([
            ("system", system_text),
            ("human", human_text),
        ])
    else:
        return ChatPromptTemplate.from_messages([
            ("system", content),
        ])
