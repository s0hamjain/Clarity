"""
title: MovingAround (Manim CE docs)
description: A blue square shifts sideways, changes color, shrinks, and rotates, one animation after another using .animate.
category: general
tags: Square, animate, shift, set_fill, scale, rotate
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        square = Square(color=BLUE, fill_opacity=1)

        self.play(square.animate.shift(LEFT))
        self.play(square.animate.set_fill(ORANGE))
        self.play(square.animate.scale(0.3))
        self.play(square.animate.rotate(0.4))
