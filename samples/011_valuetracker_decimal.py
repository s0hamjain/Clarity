"""
title: ValueTracker-driven DecimalNumber
description: A large decimal counter smoothly animates from one value to another, driven by a ValueTracker rather than jumping between frames.
category: math
tags: ValueTracker, DecimalNumber, always_redraw
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        tracker = ValueTracker(0)
        counter = always_redraw(
            lambda: DecimalNumber(tracker.get_value(), num_decimal_places=2).scale(2)
        )
        caption = Text("Approaching pi", color=GRAY).scale(0.5).to_edge(DOWN)

        self.play(FadeIn(counter), FadeIn(caption))
        self.wait(0.3)
        self.play(tracker.animate.set_value(3.14159), run_time=3, rate_func=smooth)
        self.wait(1)
