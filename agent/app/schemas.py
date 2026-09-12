from typing import Literal, Optional
from pydantic import BaseModel, Field


# --- Vision / Intake ---
class VisionRequest(BaseModel):
    image_b64: str = Field(..., description="Raw base64 string of screenshot")
    media_type: str = Field("image/png", description="MIME type e.g. image/png")
    guardrails: bool = Field(False, description="Guardrails flag")


class VisionResponse(BaseModel):
    problem_text: str = Field(..., description="Verbatim problem transcription")
    category: Literal["math", "algorithm", "unknown"] = Field(..., description="Problem category")
    confidence: float = Field(1.0, ge=0.0, le=1.0, description="Confidence score")


# --- Explainer ---
class Scene(BaseModel):
    index: int
    narration: str
    visual: str
    duration_seconds: float = 8.0


class Storyboard(BaseModel):
    title: str
    scenes: list[Scene]


class ExplainRequest(BaseModel):
    problem_text: str
    category: str
    user_prompt: str = ""
    guardrails: bool = False


class ExplainResponse(BaseModel):
    explanation: str
    storyboard: Storyboard
    revisions: int = 0


# --- Manim Generator / Scenes ---
class SceneRenderRequest(BaseModel):
    job_id: str
    scene: Scene
    storyboard_title: str
    category: str
    guardrails: bool = False
    work_dir: str
    quality: str = "-ql"


class SceneRenderResponse(BaseModel):
    ok: bool
    clip_path: Optional[str] = None
    attempts: int = 1
    lint_retries: int = 0
    snippets_used: list[str] = []
    snippet_id: Optional[str] = None
    stage: Optional[str] = None
    last_traceback: Optional[str] = None


# --- Snippets / Corpus ---
class SnippetIngestRequest(BaseModel):
    title: str
    description: str
    category: str
    tags: list[str] = []
    source: str
    origin: str = "generated"
    verified: bool = False


class SnippetIngestResponse(BaseModel):
    id: str
    embedded: bool = True


class SnippetItem(BaseModel):
    id: str
    title: str
    category: str
    origin: str
    verified: bool
    created_at: str


class SnippetListResponse(BaseModel):
    snippets: list[SnippetItem]
    next_cursor: Optional[str] = None


class SnippetDetailResponse(BaseModel):
    id: str
    title: str
    description: str
    category: str
    tags: list[str] = []
    source: str
    origin: str
    verified: bool
    created_at: str


class SnippetPatchRequest(BaseModel):
    title: Optional[str] = None
    description: Optional[str] = None
    category: Optional[str] = None
    tags: Optional[list[str]] = None
    verified: Optional[bool] = None


class SnippetSearchRequest(BaseModel):
    scene: Scene
    category: str
    k: int = 3
    hint: str = ""


class SearchResultItem(BaseModel):
    id: str
    title: str
    description: str
    source: str
    score: float


class SnippetSearchResponse(BaseModel):
    snippets: list[SearchResultItem]


# --- Codegen (Debug) ---
class CodegenSnippet(BaseModel):
    title: str
    source: str


class CodegenRequest(BaseModel):
    scene: Scene
    snippets: list[CodegenSnippet] = []
    previous_source: Optional[str] = None
    traceback: Optional[str] = None
    guardrails: bool = False


class CodegenResponse(BaseModel):
    manim_source: str
    scene_class: str = "GeneratedScene"


# --- Health ---
class HealthResponse(BaseModel):
    ok: bool
    gemini: bool
    anthropic: bool
    voyage: bool
    atlas: bool
    coordinator: bool
    snippets_verified: int
    graphs: list[str]
    models: dict[str, str]
    version: str = "0.1.0"
