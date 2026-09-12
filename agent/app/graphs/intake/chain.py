from langchain_core.messages import HumanMessage
from app.llm import gemini_flash_deterministic
from app.prompts import load_prompt
from app.schemas import VisionResponse


def run_intake(image_b64: str, media_type: str = "image/png", guardrails: bool = False, user_prompt: str = "") -> VisionResponse:
    """Intake chain: reads the screenshot (verbatim transcription if it's written text, a
    precise description if it's a diagram/graph/plot) using Gemini Flash at temperature=0
    with structured output. user_prompt is the user's own words about what they want to
    visualize or understand — the only signal available when the image itself has no
    written "problem" on it, e.g. a bare graph diagram."""
    prompt = load_prompt("vision")
    llm = gemini_flash_deterministic().with_structured_output(VisionResponse)

    # Format base64 image data URL for LangChain content block
    if not image_b64.startswith("data:"):
        image_url = f"data:{media_type};base64,{image_b64}"
    else:
        image_url = image_b64

    content = [
        {
            "type": "image_url",
            "image_url": {"url": image_url},
        }
    ]
    if user_prompt.strip():
        content.append({
            "type": "text",
            "text": f"What the user wants help visualizing or understanding: {user_prompt.strip()}",
        })

    human_msg = HumanMessage(content=content)

    system_messages = prompt.format_messages()
    messages = system_messages + [human_msg]

    response: VisionResponse = llm.invoke(messages)
    return response
