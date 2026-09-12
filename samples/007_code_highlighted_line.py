"""
title: Code block with a highlighted line
description: A short Python function appears as a syntax-highlighted code block; a rectangle is drawn around the buggy line to point it out.
category: algorithm
tags: Code, SurroundingRectangle
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        source = """def clamp(x, lo, hi):
    if x < lo:
        return lo
    if x > hi:
        return lo
    return x"""

        code = Code(
            code_string=source,
            language="python",
            background="window",
        )
        code.scale(0.8)

        self.play(FadeIn(code))
        self.wait(0.5)

        buggy_line = code.code_lines[3]
        box = SurroundingRectangle(buggy_line, color=RED, buff=0.1)
        self.play(Create(box))
        self.wait(1.5)
