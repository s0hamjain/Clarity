"""
title: Matrix with a row highlighted
description: A 3x3 matrix fades in, then a rectangle is drawn around one row to draw attention to it.
category: math
tags: Matrix, SurroundingRectangle
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        matrix = Matrix([[1, 2, 3], [4, 5, 6], [7, 8, 9]])

        self.play(FadeIn(matrix))
        self.wait(0.5)

        row = VGroup(*matrix.get_rows()[1])
        box = SurroundingRectangle(row, color=YELLOW, buff=0.2)
        self.play(Create(box))
        self.wait(1.5)
