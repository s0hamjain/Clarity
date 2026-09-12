"""
title: PointMovingOnShapes (Manim CE docs)
description: A dot grows into view, transforms into a copy of itself, moves along a circle's path, then rotates about a point.
category: general
tags: Circle, Dot, MoveAlongPath, Rotating, Transform
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        circle = Circle(radius=1, color=BLUE)
        dot = Dot()
        dot2 = dot.copy().shift(RIGHT)
        self.add(dot)

        line = Line([3, 0, 0], [5, 0, 0])
        self.add(line)

        self.play(GrowFromCenter(circle))
        self.play(Transform(dot, dot2))
        self.play(MoveAlongPath(dot, circle), run_time=2, rate_func=linear)
        self.play(Rotating(dot, about_point=[2, 0, 0]), run_time=1.5)
        self.wait()
