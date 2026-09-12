"""
title: VectorArrow (Manim CE docs)
description: A vector arrow drawn on a coordinate plane, with its origin and tip labeled by their coordinates.
category: math
tags: Arrow, NumberPlane, Dot, Text
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        dot = Dot(ORIGIN)
        arrow = Arrow(ORIGIN, [2, 2, 0], buff=0)
        numberplane = NumberPlane()
        origin_text = Text('(0, 0)').next_to(dot, DOWN)
        tip_text = Text('(2, 2)').next_to(arrow.get_end(), RIGHT)
        self.add(numberplane, dot, arrow, origin_text, tip_text)
