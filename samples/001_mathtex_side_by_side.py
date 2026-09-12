"""
title: MathTex with relative positioning, then a Transform
description: A function is written on screen, labeled with a derivative operator above it, then transforms in place into its derivative.
category: math
tags: MathTex, next_to, Transform, Write
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        original = MathTex("f(x) = x^2 \\sin x")
        derivative = MathTex("f'(x) = 2x \\sin x + x^2 \\cos x")
        label = MathTex("\\frac{d}{dx}")

        original.to_edge(UP, buff=1.5)
        label.next_to(original, UP, buff=0.4)
        derivative.move_to(original)

        self.play(Write(original))
        self.wait(0.5)
        self.play(FadeIn(label, shift=DOWN * 0.2))
        self.wait(0.5)
        self.play(Transform(original, derivative))
        self.wait(1)
