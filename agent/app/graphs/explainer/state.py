from typing import Optional
from pydantic import BaseModel, Field
from app.schemas import Storyboard


class ExplainDraft(BaseModel):
    explanation: str = Field(..., description="Step-by-step written explanation in Markdown format")
    storyboard: Storyboard = Field(..., description="Animation storyboard containing 2-5 scenes")


class Critique(BaseModel):
    passed: bool = Field(..., description="True if the draft passes all quality and storyboard rules")
    issues: list[str] = Field(default_factory=list, description="List of rule violations or quality issues identified")


class ExplainerState(BaseModel):
    problem_text: str
    category: str
    user_prompt: str = ""
    guardrails: bool = False
    draft: Optional[ExplainDraft] = None
    critique: Optional[Critique] = None
    revisions: int = 0
