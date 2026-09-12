import logging
from langgraph.graph import StateGraph, START, END
from app.graphs.manim_generator.state import RenderState
from app.graphs.manim_generator.nodes import (
    retrieve_node,
    generate_node,
    lint_node,
    render_node,
    ingest_node,
)

logger = logging.getLogger(__name__)


def check_lint(state: RenderState) -> str:
    """Conditional edge after lint node."""
    if state.traceback is None:
        return "render"

    if state.attempts >= 3:
        logger.warning(f"Exceeded max total attempts ({state.attempts}/3). Terminating graph.")
        return END

    if state.lint_retries == 0:
        logger.info(f"Max lint retries reached. Triggering re-retrieval for attempt #{state.attempts + 1}.")
        return "retrieve"

    logger.info(f"Lint failed. Triggering lint retry #{state.lint_retries}. Reason: {state.traceback}")
    return "generate"


def check_render(state: RenderState) -> str:
    """Conditional edge after render node."""
    if state.clip_path:
        return "ingest"

    if state.attempts < 3:
        logger.info(f"Render failed (attempt {state.attempts}/3). Retrying with hint: '{state.hint}'")
        return "retrieve"
    else:
        logger.warning(f"Exceeded max render attempts ({state.attempts}/3). Giving up on the render.")
        return END


builder = StateGraph(RenderState)
builder.add_node("retrieve", retrieve_node)
builder.add_node("generate", generate_node)
builder.add_node("lint", lint_node)
builder.add_node("render", render_node)
builder.add_node("ingest", ingest_node)

builder.add_edge(START, "retrieve")
builder.add_edge("retrieve", "generate")
builder.add_edge("generate", "lint")

builder.add_conditional_edges("lint", check_lint, {"render": "render", "generate": "generate", "retrieve": "retrieve", END: END})
builder.add_conditional_edges("render", check_render, {"ingest": "ingest", "retrieve": "retrieve", END: END})

builder.add_edge("ingest", END)

manim_generator_graph = builder.compile()
