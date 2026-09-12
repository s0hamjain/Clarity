"""
title: MovingGroupToDestination (Manim CE docs)
description: A group of four dots moves together so that one specific dot in the group lands exactly on a target point.
category: general
tags: VGroup, Dot, animate, shift
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        group = VGroup(Dot(LEFT), Dot(ORIGIN), Dot(RIGHT, color=RED), Dot(2 * RIGHT)).scale(1.4)
        dest = Dot([4, 3, 0], color=YELLOW)
        self.add(group, dest)
        self.play(group.animate.shift(dest.get_center() - group[2].get_center()))
        self.wait(0.5)
