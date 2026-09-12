from typing import Optional
from pydantic import BaseModel, Field
from app.schemas import Scene, CodegenSnippet


class ManimSource(BaseModel):
    manim_source: str = Field(..., description="Executable Python Manim CE code for GeneratedScene")
    scene_class: str = Field("GeneratedScene", description="Must be 'GeneratedScene'")


class SceneState(BaseModel):
    job_id: str
    scene: Scene
    storyboard_title: str
    category: str
    guardrails: bool = False
    work_dir: str
    quality: str = "-ql"

    snippets: list[CodegenSnippet] = Field(default_factory=list)
    hint: str = ""

    source: Optional[str] = None
    traceback: Optional[str] = None

    attempts: int = 0
    lint_retries: int = 0

    clip_path: Optional[str] = None
    stage: Optional[str] = None
    snippet_id: Optional[str] = None
    snippets_used: list[str] = Field(default_factory=list)
