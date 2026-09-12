"""
title: Table of values
description: A small table of x and f(x) values fades in, then one cell is highlighted to draw attention to it.
category: math
tags: Table, Indicate
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        table = Table(
            [["0", "0"], ["1", "1"], ["2", "4"], ["3", "9"]],
            col_labels=[Text("x"), Text("f(x)")],
        )
        table.scale(0.8)

        self.play(Create(table))
        self.wait(0.5)
        self.play(Indicate(table.get_entries((3, 2)), color=YELLOW))
        self.wait(1)
