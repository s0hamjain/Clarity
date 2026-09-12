"""
title: Binary tree with in-order traversal
description: A small binary tree of circles connected by lines; nodes light up one at a time in in-order traversal sequence.
category: algorithm
tags: VGroup, Line, Indicate
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        def node(label):
            circle = Circle(radius=0.35, color=BLUE)
            text = Text(label).scale(0.5).move_to(circle)
            return VGroup(circle, text)

        root = node("4")
        root.to_edge(UP, buff=1.0)

        left = node("2")
        right = node("6")
        VGroup(left, right).arrange(RIGHT, buff=2.5).next_to(root, DOWN, buff=1.0)

        left_left = node("1")
        left_right = node("3")
        VGroup(left_left, left_right).arrange(RIGHT, buff=0.8).next_to(left, DOWN, buff=1.0)

        edges = VGroup(
            Line(root.get_bottom(), left.get_top()),
            Line(root.get_bottom(), right.get_top()),
            Line(left.get_bottom(), left_left.get_top()),
            Line(left.get_bottom(), left_right.get_top()),
        )

        tree = VGroup(root, left, right, left_left, left_right)

        self.play(Create(edges), FadeIn(tree))
        self.wait(0.5)

        in_order = [left_left, left, left_right, root, right]
        for n in in_order:
            self.play(Indicate(n, color=YELLOW), run_time=0.5)
        self.wait(1)
