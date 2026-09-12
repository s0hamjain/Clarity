"""
title: Graph with BFS-style highlighting
description: A small graph of nodes and edges appears; nodes light up one by one in breadth-first order starting from a root.
category: algorithm
tags: Graph, Indicate
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        vertices = [1, 2, 3, 4, 5]
        edges = [(1, 2), (1, 3), (2, 4), (3, 5)]
        graph = Graph(
            vertices,
            edges,
            layout="tree",
            root_vertex=1,
            vertex_config={"radius": 0.3, "fill_color": BLUE},
            labels=True,
        )
        graph.scale(1.2)

        self.play(Create(graph))
        self.wait(0.3)

        bfs_order = [1, 2, 3, 4, 5]
        for v in bfs_order:
            self.play(Indicate(graph.vertices[v], color=YELLOW), run_time=0.5)
        self.wait(1)
