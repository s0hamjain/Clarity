"""
title: An angle sweeping between two rays
description: Two rays share a vertex; an arc sweeps from one ray to the other while a label shows the angle growing.
category: math
tags: Angle, Line, ValueTracker, always_redraw
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        ray_a = Line(ORIGIN, RIGHT * 3)
        tracker = ValueTracker(0)
        ray_b = always_redraw(lambda: Line(ORIGIN, RIGHT * 3).rotate(tracker.get_value(), about_point=ORIGIN))
        angle = always_redraw(lambda: Angle(ray_a, ray_b.copy(), radius=0.8, color=YELLOW))
        label = always_redraw(
            lambda: DecimalNumber(tracker.get_value() * 180 / PI, num_decimal_places=0, unit="^\\circ")
            .scale(0.7)
            .next_to(angle, RIGHT, buff=0.2)
        )

        self.play(Create(ray_a))
        self.add(ray_b, angle, label)
        self.wait(0.3)
        self.play(tracker.animate.set_value(PI / 2), run_time=2, rate_func=smooth)
        self.wait(1.5)
