from langchain_core.messages import HumanMessage
from app.llm import gemini_flash_deterministic
from app.prompts import load_prompt
from app.schemas import VisionResponse


def run_intake(image_b64: str, media_type: str = "image/png", guardrails: bool = False) -> VisionResponse:
    """Intake chain: transcribes screenshot verbatim using Gemini Flash at temperature=0 with structured output."""
    prompt = load_prompt("vision")
    llm = gemini_flash_deterministic().with_structured_output(VisionResponse)

    # Format base64 image data URL for LangChain content block
    if not image_b64.startswith("data:"):
        image_url = f"data:{media_type};base64,{image_b64}"
    else:
        image_url = image_b64

    human_msg = HumanMessage(
        content=[
            {
                "type": "image_url",
                "image_url": {"url": image_url},
            }
        ]
    )

    system_messages = prompt.format_messages()
    messages = system_messages + [human_msg]

    response: VisionResponse = llm.invoke(messages)
    return response
