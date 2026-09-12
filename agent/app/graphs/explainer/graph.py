import logging
from langgraph.graph import StateGraph, START, END
from app.graphs.explainer.state import ExplainerState
from app.graphs.explainer.nodes import draft_node, critique_node, revise_node

logger = logging.getLogger(__name__)


def should_revise(state: ExplainerState) -> str:
    """Conditional edge after critique: pass -> END; fail & revisions < 1 -> revise; else -> END."""
    if state.critique and state.critique.passed:
        return END
    elif state.revisions < 1:
        logger.info(f"Draft failed critique. Triggering revision 1. Issues: {state.critique.issues if state.critique else []}")
        return "revise"
    else:
        logger.warning(f"Draft failed critique after maximum revisions ({state.revisions}). Giving up revision loop.")
        return END


builder = StateGraph(ExplainerState)
builder.add_node("draft", draft_node)
builder.add_node("critique", critique_node)
builder.add_node("revise", revise_node)

builder.add_edge(START, "draft")
builder.add_edge("draft", "critique")
builder.add_conditional_edges("critique", should_revise, {"revise": "revise", END: END})
builder.add_edge("revise", "critique")

explainer_graph = builder.compile()
