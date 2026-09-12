from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_anthropic import ChatAnthropic
from app.config import settings


def gemini_flash(thinking: str = "medium") -> ChatGoogleGenerativeAI:
    """Gemini Flash model for general text/explanation tasks."""
    return ChatGoogleGenerativeAI(
        model=settings.EXPLAIN_MODEL,
        google_api_key=settings.GEMINI_API_KEY,
    )


def gemini_flash_deterministic() -> ChatGoogleGenerativeAI:
    """Gemini Flash model at temperature=0 for verbatim transcription."""
    return ChatGoogleGenerativeAI(
        model=settings.VISION_MODEL,
        temperature=0,
        google_api_key=settings.GEMINI_API_KEY,
    )


def sonnet() -> ChatAnthropic:
    """Claude Sonnet model for Manim code generation and repair. Strictly no temperature."""
    return ChatAnthropic(
        model=settings.CODEGEN_MODEL,
        max_tokens=16000,
        streaming=True,
        api_key=settings.ANTHROPIC_API_KEY,
    )
