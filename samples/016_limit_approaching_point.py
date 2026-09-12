"""
title: A limit approaching a point
description: A dot slides along a curve toward a specific x-value from both sides, showing the function value converging as x approaches the point.
category: math
tags: Axes, plot, ValueTracker, Dot
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        axes = Axes(
            x_range=[-1, 3, 1],
            y_range=[-1, 5, 1],
            x_length=8,
            y_length=5,
        )
        axes.to_edge(DOWN, buff=0.6)
        curve = axes.plot(lambda x: x**2, color=BLUE)

        target_x = 2
        marker = DashedLine(
            axes.c2p(target_x, 0), axes.c2p(target_x, target_x**2), color=GRAY
        )

        tracker = ValueTracker(0)
        dot = always_redraw(lambda: Dot(axes.c2p(tracker.get_value(), tracker.get_value() ** 2), color=YELLOW))

        self.play(Create(axes), Create(curve))
        self.play(Create(marker))
        self.add(dot)
        self.wait(0.3)
        self.play(tracker.animate.set_value(target_x), run_time=2, rate_func=smooth)
        self.wait(1.5)
