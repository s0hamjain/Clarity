"""
title: Axes with a function plot and a dot sliding along the curve
description: A coordinate plane with a sine curve plotted on it; a dot slides smoothly along the curve from left to right, driven by a value tracker.
category: math
tags: Axes, plot, ValueTracker, Dot, always_redraw
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        axes = Axes(
            x_range=[0, 3 * PI, PI / 2],
            y_range=[-1.5, 1.5, 0.5],
            x_length=10,
            y_length=5,
            axis_config={"include_tip": True},
        )
        axes.to_edge(DOWN, buff=0.6)

        curve = axes.plot(lambda x: np.sin(x), color=BLUE)

        tracker = ValueTracker(0)
        dot = always_redraw(
            lambda: Dot(
                axes.c2p(tracker.get_value(), np.sin(tracker.get_value())),
                color=YELLOW,
            )
        )

        self.play(Create(axes))
        self.play(Create(curve))
        self.add(dot)
        self.wait(0.3)
        self.play(tracker.animate.set_value(3 * PI), run_time=4, rate_func=linear)
        self.wait(1)
