"""
title: Area between two plotted functions
description: Two functions are plotted on the same axes, then the region between them is filled in to show the area.
category: math
tags: Axes, plot, get_area
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        axes = Axes(
            x_range=[0, 4, 1],
            y_range=[0, 5, 1],
            x_length=8,
            y_length=5,
        )
        axes.to_edge(DOWN, buff=0.6)

        upper = axes.plot(lambda x: 0.5 * x + 1, color=BLUE)
        lower = axes.plot(lambda x: 0.3 * x, color=GREEN)
        area = axes.get_area(upper, x_range=[0, 4], bounded_graph=lower, color=YELLOW, opacity=0.5)

        self.play(Create(axes))
        self.play(Create(upper), Create(lower))
        self.wait(0.3)
        self.play(FadeIn(area))
        self.wait(1.5)
