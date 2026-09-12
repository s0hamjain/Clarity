"""
title: NumberLine with a dot sliding to a value
description: A number line appears; a dot starts at zero and slides smoothly to a target value, with a label that tracks its position.
category: math
tags: NumberLine, ValueTracker, always_redraw, Dot
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        line = NumberLine(x_range=[-5, 5, 1], length=10, include_numbers=True)

        tracker = ValueTracker(0)
        dot = always_redraw(lambda: Dot(line.n2p(tracker.get_value()), color=YELLOW))
        label = always_redraw(
            lambda: DecimalNumber(tracker.get_value(), num_decimal_places=1).next_to(dot, UP, buff=0.3)
        )

        self.play(Create(line))
        self.add(dot, label)
        self.wait(0.3)
        self.play(tracker.animate.set_value(3.5), run_time=2, rate_func=smooth)
        self.wait(1)
