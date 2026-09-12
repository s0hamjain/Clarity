"""
title: Linked list traversal
description: A row of linked-list nodes connected by arrows; a pointer moves node to node, highlighting each as it visits.
category: algorithm
tags: VGroup, arrange, Arrow, Indicate
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        values = ["A", "B", "C", "D"]
        nodes = VGroup()
        for v in values:
            box = Square(side_length=1.0)
            label = Text(v).scale(0.7).move_to(box)
            nodes.add(VGroup(box, label))
        nodes.arrange(RIGHT, buff=1.0)
        nodes.to_edge(UP, buff=1.5)

        arrows = VGroup(
            *[Arrow(nodes[i].get_right(), nodes[i + 1].get_left(), buff=0.1) for i in range(len(nodes) - 1)]
        )

        pointer_label = Text("head").scale(0.5).next_to(nodes[0], UP, buff=0.3)

        self.play(Create(nodes), Create(arrows))
        self.play(FadeIn(pointer_label))
        self.wait(0.3)

        for i, node in enumerate(nodes):
            self.play(Indicate(node, color=YELLOW), run_time=0.4)
            if i < len(nodes) - 1:
                self.play(pointer_label.animate.next_to(nodes[i + 1], UP, buff=0.3), run_time=0.4)
        self.wait(1)
