"""
title: GradientImageFromArray (Manim CE docs)
description: A gradient image built from a raw numpy array and displayed with a surrounding rectangle.
category: general
tags: ImageMobject, SurroundingRectangle, numpy
"""
from manim import *


class GeneratedScene(Scene):
    def construct(self):
        n = 256
        imageArray = np.uint8(
            [[i * 256 / n for i in range(0, n)] for _ in range(0, n)]
        )
        image = ImageMobject(imageArray).scale(2)
        image.background_rectangle = SurroundingRectangle(image, color=GREEN)
        self.add(image, image.background_rectangle)
