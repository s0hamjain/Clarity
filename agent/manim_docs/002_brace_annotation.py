"""
title: BraceAnnotation (Manim CE docs)
description: Two dots connected by a line, annotated with braces showing horizontal distance both as text and as a formula.
category: general
tags: Brace, Line, get_text, get_tex
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        dot = Dot([-2, -1, 0])
        dot2 = Dot([2, 1, 0])
        line = Line(dot.get_center(), dot2.get_center()).set_color(ORANGE)
        b1 = Brace(line)
        b1text = b1.get_text("Horizontal distance")
        b2 = Brace(line, direction=line.copy().rotate(PI / 2).get_unit_vector())
        b2text = b2.get_tex("x-x_1")
        self.add(line, dot, dot2, b1, b2, b1text, b2text)
