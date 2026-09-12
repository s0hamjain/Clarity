from manim import *


class GeneratedScene(Scene):
    def construct(self):
        t = MathTex(r"\frac{d}{dx}\left[x^2 \sin x\right] = 2x\sin x + x^2\cos x")
        self.play(Write(t))
        self.wait(1)
