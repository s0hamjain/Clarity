"""
title: Shape morph — circle to square to triangle
description: A circle smoothly transforms into a square, then the square transforms into a triangle, each shape centered on screen.
category: general
tags: Circle, Square, Triangle, Transform
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        circle = Circle(radius=1.5, color=BLUE, fill_opacity=0.5)
        square = Square(side_length=2.6, color=GREEN, fill_opacity=0.5)
        triangle = Triangle(color=RED, fill_opacity=0.5).scale(2)

        self.play(Create(circle))
        self.wait(0.5)
        self.play(Transform(circle, square))
        self.wait(0.5)
        self.play(Transform(circle, triangle))
        self.wait(1)
