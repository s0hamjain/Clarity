"""
title: Array walk with a moving pointer
description: A row of boxes built with VGroup.arrange; an arrow pointer steps left to right beneath the row, highlighting each box as it visits it.
category: algorithm
tags: VGroup, arrange, Arrow, Indicate, next_to
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        values = [4, 1, 7, 3, 9, 2]
        boxes = VGroup()
        for v in values:
            box = Square(side_length=1.0)
            label = Text(str(v)).scale(0.7).move_to(box)
            boxes.add(VGroup(box, label))
        boxes.arrange(RIGHT, buff=0.15)
        boxes.to_edge(UP, buff=1.5)

        pointer = Arrow(UP, DOWN, buff=0, color=YELLOW).scale(0.4)
        pointer.next_to(boxes[0], DOWN, buff=0.2)

        self.play(Create(boxes))
        self.play(FadeIn(pointer))
        self.wait(0.3)

        for i, cell in enumerate(boxes):
            self.play(Indicate(cell, color=YELLOW), run_time=0.4)
            if i < len(boxes) - 1:
                self.play(pointer.animate.next_to(boxes[i + 1], DOWN, buff=0.3), run_time=0.4)
        self.wait(1)
