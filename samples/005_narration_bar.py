"""
title: Narration bar with per-beat text swaps
description: A line of narration text sits pinned to the bottom edge and is replaced with new text for each beat via fade out and fade in, while a shape animates above it.
category: general
tags: to_edge, FadeOut, FadeIn, Text
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        square = Square(side_length=1.5, color=BLUE)

        beats = [
            "We start with a square.",
            "Now it grows.",
            "And it spins.",
        ]

        narration = Text(beats[0]).scale(0.6).to_edge(DOWN, buff=0.6)

        self.play(FadeIn(square))
        self.play(FadeIn(narration))
        self.wait(1)

        self.play(square.animate.scale(1.6))
        next_narration = Text(beats[1]).scale(0.6).to_edge(DOWN, buff=0.6)
        self.play(FadeOut(narration), FadeIn(next_narration))
        narration = next_narration
        self.wait(1)

        self.play(Rotate(square, angle=PI))
        next_narration = Text(beats[2]).scale(0.6).to_edge(DOWN, buff=0.6)
        self.play(FadeOut(narration), FadeIn(next_narration))
        self.wait(1)
