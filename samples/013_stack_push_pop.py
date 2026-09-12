"""
title: Stack with push and pop
description: Boxes stack upward one at a time as values are pushed, then the top box is removed to show a pop.
category: algorithm
tags: VGroup, next_to, FadeOut
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        base = Line(LEFT * 1.5, RIGHT * 1.5).to_edge(DOWN, buff=1.5)
        self.play(Create(base))

        def make_box(value):
            box = Square(side_length=1.0)
            label = Text(str(value)).scale(0.7).move_to(box)
            return VGroup(box, label)

        stack = VGroup()
        for value in [1, 2, 3]:
            box = make_box(value)
            if len(stack) == 0:
                box.next_to(base, UP, buff=0.0)
            else:
                box.next_to(stack[-1], UP, buff=0.0)
            stack.add(box)
            self.play(FadeIn(box, shift=UP * 0.3), run_time=0.5)

        self.wait(0.5)
        top = stack[-1]
        stack.remove(top)
        self.play(FadeOut(top, shift=UP * 0.3), run_time=0.5)
        self.wait(1)
