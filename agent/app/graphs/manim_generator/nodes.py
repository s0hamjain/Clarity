import logging
from datetime import datetime, timezone
from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.documents import Document
from app.llm import sonnet
from app.prompts import load_prompt
from app.schemas import CodegenSnippet
from app.vectorstore import retriever, get_vectorstore
from app.graphs.manim_generator.state import RenderState, ManimSource
from app.graphs.manim_generator.lint import lint_manim_code
from app.graphs.manim_generator.tools import render_tool

logger = logging.getLogger(__name__)


def _storyboard_text(state: RenderState) -> str:
    """One block of text describing every beat, in order — used both as the
    retrieval query and folded into the generate prompt."""
    lines = [state.storyboard_title]
    for s in state.scenes:
        lines.append(f"{s.narration} {s.visual}")
    return "\n".join(lines)


def retrieve_node(state: RenderState) -> dict:
    """Retrieves verified Manim snippets from Atlas matching the whole storyboard.

    k is higher than the old per-scene k=3 (now 6) because one script has to
    cover several distinct techniques — a plot AND a transform AND text,
    say — rather than just one.
    """
    query_text = f"{_storyboard_text(state)} {state.hint}".strip()
    try:
        docs = retriever(state.category, k=6).invoke(query_text)
        snippets = []
        snippets_used = []
        for d in docs:
            snippet_id = str(d.metadata.get("_id", d.metadata.get("title", "")))
            title = d.metadata.get("title", "Reference Snippet")
            source = d.metadata.get("source", "")
            if source:
                snippets.append(CodegenSnippet(title=title, source=source))
                snippets_used.append(snippet_id)
        return {"snippets": snippets, "snippets_used": snippets_used}
    except Exception as e:
        logger.warning(f"Retrieval failed or vector index offline: {e}")
        return {"snippets": [], "snippets_used": []}


def generate_node(state: RenderState) -> dict:
    """Generates or repairs one continuous Manim script covering every scene."""
    is_repair = bool(state.traceback)
    prompt_name = "repair" if is_repair else "codegen"
    prompt_template = load_prompt(prompt_name)

    system_text = prompt_template.format_messages()[0].content

    if state.snippets:
        system_text += "\n\nReference Examples — imitate these patterns:\n"
        for idx, snip in enumerate(state.snippets, 1):
            system_text += f"\n--- Example {idx}: {snip.title} ---\n```python\n{snip.source}\n```\n"

    beats = "\n".join(
        f"Beat {s.index} ({s.duration_seconds}s) — Narration: {s.narration} | Visual: {s.visual}"
        for s in state.scenes
    )

    if is_repair:
        tb_snippet = (state.traceback or "")[-1500:]
        human_text = (
            f"Previous Code:\n```python\n{state.source or ''}\n```\n\n"
            f"Render Failure Traceback:\n{tb_snippet}\n\n"
            f"Fix the minimal bug to make this scene render cleanly. Maintain class GeneratedScene(Scene)."
        )
    else:
        human_text = (
            f"Storyboard Title: {state.storyboard_title}\n"
            f"Guardrails Active: {state.guardrails}\n\n"
            f"Beats, in order — play them as one continuous animation, not separate scenes:\n{beats}\n\n"
            f"Write one complete `class GeneratedScene(Scene):` whose construct() plays every "
            f"beat above in sequence within a single flow."
        )

    messages = [
        SystemMessage(content=system_text),
        HumanMessage(content=human_text),
    ]

    llm = sonnet().with_structured_output(ManimSource)
    res: ManimSource = llm.invoke(messages)

    if res.scene_class != "GeneratedScene":
        logger.warning(f"Model returned invalid scene_class '{res.scene_class}' instead of 'GeneratedScene'")
        return {"source": res.manim_source, "traceback": f"invalid scene_class '{res.scene_class}': must be 'GeneratedScene'"}

    return {"source": res.manim_source}


def lint_node(state: RenderState) -> dict:
    """Statically lints the generated script using AST."""
    reason = lint_manim_code(state.source or "")
    if reason:
        logger.info(f"Lint failed (retry {state.lint_retries + 1}): {reason}")
        if state.lint_retries >= 1:
            logger.warning(
                f"Exceeded max lint retries ({state.lint_retries + 1}). "
                f"Counting as render attempt #{state.attempts + 1} and resetting lint_retries."
            )
            return {
                "traceback": f"Lint Error: {reason}",
                "lint_retries": 0,
                "attempts": state.attempts + 1,
            }
        return {
            "traceback": f"Lint Error: {reason}",
            "lint_retries": state.lint_retries + 1,
        }
    return {"traceback": None}


def render_node(state: RenderState) -> dict:
    """Invokes the coordinator's internal render tool inside a Docker container."""
    res = render_tool(
        source=state.source or "",
        work_dir=state.work_dir,
        quality=state.quality,
    )
    attempts = state.attempts + 1

    if res.ok:
        return {
            "attempts": attempts,
            "clip_path": res.clip_path,
            "stage": None,
            "traceback": None,
        }
    tb_first_line = res.traceback.splitlines()[0] if res.traceback else "Render failed"
    return {
        "attempts": attempts,
        "stage": res.stage or "render",
        "traceback": res.traceback,
        "hint": tb_first_line,
    }


def ingest_node(state: RenderState) -> dict:
    """Ingests a successful script back into Atlas as an unverified snippet."""
    if not (state.clip_path and state.source):
        return {"snippet_id": None}

    try:
        store = get_vectorstore()
        title = state.storyboard_title
        desc = " ".join(s.visual for s in state.scenes)
        doc_content = f"{title}\n{desc}\nGenerated Scene"

        doc = Document(
            page_content=doc_content,
            metadata={
                "title": title,
                "description": desc,
                "category": state.category,
                "tags": ["generated"],
                "source": state.source,
                "origin": "generated",
                "verified": False,
                "created_at": datetime.now(timezone.utc).isoformat(),
            },
        )
        ids = store.add_documents([doc])
        snippet_id = ids[0] if ids else None
        return {"snippet_id": snippet_id}
    except Exception as e:
        logger.warning(f"Snippet ingest failed (non-fatal): {e}")
        return {"snippet_id": None}
