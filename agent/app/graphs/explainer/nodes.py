import logging
from langchain_core.messages import HumanMessage, SystemMessage
from app.llm import gemini_flash
from app.prompts import load_prompt
from app.graphs.explainer.state import ExplainerState, ExplainDraft, Critique

logger = logging.getLogger(__name__)


def draft_node(state: ExplainerState) -> dict:
    """Drafts initial explanation and storyboard using Gemini Flash."""
    category = state.category.lower()
    prompt_name = "explain_math" if category == "math" else "explain_algorithm"
    prompt_template = load_prompt(prompt_name)

    system_text = prompt_template.format_messages()[0].content
    if state.guardrails:
        guardrails_template = load_prompt("explain_guardrails")
        system_text += "\n\n" + guardrails_template.format_messages()[0].content

    messages = [
        SystemMessage(content=system_text),
        HumanMessage(content=f"Problem:\n{state.problem_text}\n\nUser Question/Context:\n{state.user_prompt or 'Explain this problem step by step.'}"),
    ]

    llm = gemini_flash().with_structured_output(ExplainDraft)
    draft: ExplainDraft = llm.invoke(messages)
    return {"draft": draft}


def critique_node(state: ExplainerState) -> dict:
    """Critiques draft explanation and storyboard against Python hard rules first, then Gemini rubric."""
    draft = state.draft
    if not draft or not draft.storyboard:
        return {"critique": Critique(passed=False, issues=["Missing draft or storyboard"])}

    scenes = draft.storyboard.scenes
    issues = []

    # 1. Python Hard Rule Checks
    if not (2 <= len(scenes) <= 5):
        issues.append(f"Storyboard must contain 2 to 5 scenes; found {len(scenes)} scenes.")

    for s in scenes:
        if len(s.narration) > 90:
            issues.append(f"Scene {s.index} narration exceeds 90 characters ({len(s.narration)} chars): '{s.narration}'")
        if not (5 <= s.duration_seconds <= 15):
            issues.append(f"Scene {s.index} duration ({s.duration_seconds}s) is outside allowed range 5–15 seconds.")

    if issues:
        logger.info(f"Python hard-rules critique failed with {len(issues)} issues")
        return {"critique": Critique(passed=False, issues=issues)}

    # 2. LLM Critique Rubric Check
    critique_prompt = load_prompt("critique")
    system_text = critique_prompt.format_messages()[0].content

    human_content = (
        f"Problem Statement:\n{state.problem_text}\n\n"
        f"Guardrails Active: {state.guardrails}\n\n"
        f"Draft Explanation:\n{draft.explanation}\n\n"
        f"Draft Storyboard:\nTitle: {draft.storyboard.title}\n"
    )
    for s in scenes:
        human_content += f"- Scene {s.index} ({s.duration_seconds}s): Narration: '{s.narration}' | Visual: '{s.visual}'\n"

    messages = [
        SystemMessage(content=system_text),
        HumanMessage(content=human_content),
    ]

    llm = gemini_flash(thinking="low").with_structured_output(Critique)
    critique: Critique = llm.invoke(messages)
    return {"critique": critique}


def revise_node(state: ExplainerState) -> dict:
    """Revises draft explanation and storyboard based on critique feedback."""
    category = state.category.lower()
    prompt_name = "explain_math" if category == "math" else "explain_algorithm"
    prompt_template = load_prompt(prompt_name)

    system_text = prompt_template.format_messages()[0].content
    if state.guardrails:
        guardrails_template = load_prompt("explain_guardrails")
        system_text += "\n\n" + guardrails_template.format_messages()[0].content

    issues_text = "\n".join(f"- {issue}" for issue in (state.critique.issues if state.critique else []))

    messages = [
        SystemMessage(content=system_text),
        HumanMessage(
            content=(
                f"Problem:\n{state.problem_text}\n\n"
                f"User Question/Context:\n{state.user_prompt or 'Explain this problem step by step.'}\n\n"
                f"Previous draft failed critique with the following issues:\n{issues_text}\n\n"
                f"Please revise the explanation and storyboard to fix ALL identified issues."
            )
        ),
    ]

    llm = gemini_flash().with_structured_output(ExplainDraft)
    revised_draft: ExplainDraft = llm.invoke(messages)
    return {
        "draft": revised_draft,
        "revisions": state.revisions + 1,
    }
