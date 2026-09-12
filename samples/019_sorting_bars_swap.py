"""
title: Sorting bars with a swap
description: A row of bars of different heights represents an unsorted array; two adjacent bars out of order swap places.
category: algorithm
tags: VGroup, arrange, Swap
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        heights = [2, 4, 1, 3]
        bars = VGroup()
        for h in heights:
            bar = Rectangle(width=0.8, height=h, fill_color=BLUE, fill_opacity=0.8)
            bars.add(bar)
        bars.arrange(RIGHT, buff=0.3, aligned_edge=DOWN)
        bars.to_edge(DOWN, buff=1.0)

        labels = VGroup(*[Text(str(h)).scale(0.5).next_to(bar, UP, buff=0.1) for h, bar in zip(heights, bars)])

        self.play(Create(bars), FadeIn(labels))
        self.wait(0.5)

        # bars[0] (height 2) and bars[1] (height 4) are out of order — swap them.
        left_pos = bars[0].get_center()
        right_pos = bars[1].get_center()
        self.play(
            bars[0].animate.move_to(right_pos),
            bars[1].animate.move_to(left_pos),
            labels[0].animate.move_to(labels[1].get_center()),
            labels[1].animate.move_to(labels[0].get_center()),
            run_time=1,
        )
        self.wait(1.5)
