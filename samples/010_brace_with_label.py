"""
title: Brace with a label
description: Two points on a line are spanned by a curly brace, labeled with the distance between them.
category: math
tags: Brace, Line
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        line = Line(LEFT * 3, RIGHT * 3)
        dot_a = Dot(line.get_start())
        dot_b = Dot(line.get_end())

        brace = Brace(line, DOWN)
        label = brace.get_text("6 units")

        shorter_line = Line(LEFT * 1.5, RIGHT * 1.5)
        shorter_brace = Brace(shorter_line, DOWN)
        shorter_label = shorter_brace.get_text("3 units")

        self.play(Create(line), FadeIn(dot_a), FadeIn(dot_b))
        self.wait(0.3)
        self.play(GrowFromCenter(brace), FadeIn(label))
        self.wait(1)
        self.play(
            Transform(line, shorter_line),
            Transform(brace, shorter_brace),
            Transform(label, shorter_label),
        )
        self.wait(1.5)
