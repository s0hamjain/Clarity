from fastapi import APIRouter
from app.schemas import CodegenRequest, CodegenResponse

router = APIRouter()


@router.post("/codegen", response_model=CodegenResponse)
async def debug_codegen(req: CodegenRequest):
    """POST /codegen - Debug endpoint running generate node alone (Stubbed for Sprint 1)."""
    return CodegenResponse(
        manim_source="""from manim import *

class GeneratedScene(Scene):
    def construct(self):
        t = Text("Hello Clarity")
        self.play(Write(t))
        self.wait(1)
""",
        scene_class="GeneratedScene",
    )
