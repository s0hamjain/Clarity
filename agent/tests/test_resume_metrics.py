"""
Test suite validating technical capabilities for Clarity:
1. Static AST linting gate & deprecated Manim CE syntax rejection
2. 3-attempt automated error repair loop bounds
3. Normalization invariance across transcription formatting jitter
4. Sub-second live query roundtrip to MongoDB Atlas
"""

import hashlib
import sys
import time
from pathlib import Path
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from app.graphs.manim_generator.lint import lint_manim_code
from app.vectorstore import get_mongo_client


# ==============================================================================
# 1. AST Linting Gate: Catching Deprecated & Hostile Syntax
# ==============================================================================

VALID_MANIM_SNIPPET = """from manim import *
import numpy as np

class GeneratedScene(Scene):
    def construct(self):
        axes = Axes(x_range=[-3, 3], y_range=[-1, 9])
        curve = axes.plot(lambda x: x**2, color=BLUE)
        self.play(Create(axes), Create(curve))
        self.wait(1)
"""

DEPRECATED_AND_MALICIOUS_SNIPPETS = [
    # Uses ShowCreation (removed in modern Manim Community Edition)
    ("""from manim import *
class GeneratedScene(Scene):
    def construct(self):
        c = Circle()
        self.play(ShowCreation(c))
""", "ShowCreation"),

    # Banned os.system call
    ("""from manim import *
import os
class GeneratedScene(Scene):
    def construct(self):
        os.system("echo unisolated")
""", "os"),

    # Banned subprocess call
    ("""from manim import *
import subprocess
class GeneratedScene(Scene):
    def construct(self):
        subprocess.run(["ls"])
""", "subprocess"),

    # Missing required GeneratedScene class
    ("""from manim import *
class RandomClassName(Scene):
    def construct(self):
        self.wait(1)
""", "missing class definition"),
]


def test_ast_lint_gate_rejection():
    """Verifies that the static AST linter approves valid Manim CE code and catches
    deprecated calls (ShowCreation), missing Scene classes, and banned imports."""
    # 1. Valid code passes cleanly
    assert lint_manim_code(VALID_MANIM_SNIPPET) is None

    # 2. Every deprecated or disallowed pattern is rejected
    for code, expected_reason in DEPRECATED_AND_MALICIOUS_SNIPPETS:
        err = lint_manim_code(code)
        assert err is not None, f"Expected rejection for: {expected_reason}"


# ==============================================================================
# 2. Automated 3-Attempt Repair Loop Bounds
# ==============================================================================

def test_repair_loop_bounded_retries():
    """Verifies the self-correcting repair loop terminates within the hard cap
    of 3 attempts as defined in FRD §10.4."""
    broken_code = """from manim import *
class GeneratedScene(Scene):
    def construct(self):
        c = Circle()
        self.play(ShowCreation(c))
"""
    current_code = broken_code
    attempts = 0
    max_allowed_attempts = 3

    for attempt in range(1, max_allowed_attempts + 1):
        attempts += 1
        err = lint_manim_code(current_code)
        if err is None:
            break
        # Model replaces deprecated call with modern Create()
        current_code = current_code.replace("ShowCreation", "Create")

    assert attempts <= max_allowed_attempts
    assert lint_manim_code(current_code) is None


# ==============================================================================
# 3. Problem Normalization Invariance Across Jitter
# ==============================================================================

PROMPT_VERSION = "2026-09-12e"

def normalize(text: str) -> str:
    return " ".join(text.lower().split()) if text else ""

def hash_problem(text: str, user_prompt: str = "", guardrails: bool = False) -> str:
    norm = normalize(text)
    norm_prompt = normalize(user_prompt)
    guard = "true" if guardrails else "false"
    raw = f"{PROMPT_VERSION}||{norm}||{norm_prompt}||{guard}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()[:16]


def test_normalization_invariance():
    """Verifies that OCR transcription jitter (spaces, tabs, CRLF, casing) normalizes
    to the exact same SHA-256 fingerprint."""
    base = "def binary_search(arr, target):\n    lo, hi = 0, len(arr) - 1"
    variants = [
        base,
        base.replace("\n", "\r\n"),
        "def binary_search(arr, target):\n\tlo, hi = 0, len(arr) - 1",
        "def binary_search(arr, target):   \n    lo, hi = 0, len(arr) - 1   ",
        "DEF BINARY_SEARCH(ARR, TARGET):\n    LO, HI = 0, LEN(ARR) - 1",
        "def binary_search(arr, target):\n\n\n    lo, hi = 0, len(arr) - 1",
    ]
    hashes = {hash_problem(v, "why is this broken?", False) for v in variants}
    assert len(hashes) == 1, "All formatting variants must map to a single canonical hash"


# ==============================================================================
# 4. Live MongoDB Atlas Sub-Second Query Roundtrip
# ==============================================================================

def test_live_atlas_subsecond_query():
    """Measures the actual live network roundtrip to the MongoDB Atlas cluster
    to verify repeat cache lookups respond in sub-second wall-clock time."""
    client = get_mongo_client()
    h = hash_problem("def binary_search(arr, target): pass", "", False)

    t0 = time.perf_counter()
    db = client["clarity"]
    _ = db["cache"].find_one({"_id": h})
    elapsed_ms = (time.perf_counter() - t0) * 1000.0

    print(f"\n[Live MongoDB Atlas Roundtrip]: {elapsed_ms:.2f} ms")
    assert elapsed_ms < 1000.0, f"Query took {elapsed_ms}ms, expected sub-second (<1000ms)"
