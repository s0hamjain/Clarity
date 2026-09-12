"""
title: Recursion tree via nested VGroups
description: A call tree for a recursive function grows top to bottom, each level's calls arranged under their parent and connected by lines.
category: algorithm
tags: VGroup, arrange, Line, Text
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        def node(label):
            circle = Circle(radius=0.35, color=BLUE)
            text = Text(label).scale(0.5).move_to(circle)
            return VGroup(circle, text)

        root = node("f(3)")
        root.to_edge(UP, buff=1.0)

        children = VGroup(node("f(2)"), node("f(1)"))
        children.arrange(RIGHT, buff=1.5)
        children.next_to(root, DOWN, buff=1.0)

        grandchildren = VGroup(node("f(1)"), node("f(0)"))
        grandchildren.arrange(RIGHT, buff=1.0)
        grandchildren.next_to(children[0], DOWN, buff=1.0)

        edges = VGroup(
            Line(root.get_bottom(), children[0].get_top()),
            Line(root.get_bottom(), children[1].get_top()),
            Line(children[0].get_bottom(), grandchildren[0].get_top()),
            Line(children[0].get_bottom(), grandchildren[1].get_top()),
        )

        self.play(FadeIn(root))
        self.play(Create(edges[0]), Create(edges[1]), FadeIn(children))
        self.wait(0.3)
        self.play(Create(edges[2]), Create(edges[3]), FadeIn(grandchildren))
        self.wait(1.5)
