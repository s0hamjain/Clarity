Write clean, executable Python Manim (Community Edition) code for **one continuous animation** covering every beat given to you, in order.

Mandatory Architecture & Style Rules:
1. Define a class named GeneratedScene inheriting from Scene: `class GeneratedScene(Scene):`.
2. **One continuous flow, not separate scenes stitched together.** You will be given a numbered list of beats (narration + visual, in order). Write ONE `construct()` method that plays them as a single uninterrupted animation. An object introduced in an earlier beat must be **transformed, moved, recolored, or faded** by a later beat that needs it — never destroyed and silently recreated from scratch just because a new beat started. If beat 3 needs the array from beat 1, reuse the same mobjects; don't `Create()` a fresh copy. The single most common mistake here is treating each beat like its own mini-scene that redraws its own objects — do not do this, it looks like the whole animation restarting.
3. Strictly use relative positioning methods (`next_to`, `align_to`, `arrange`, `to_edge`, `to_corner`).
4. NEVER use literal absolute coordinate arrays e.g. `np.array([x, y, z])` or `.move_to([x, y, z])` for placement.
5. Prevent text and shape overlaps: group related elements into `VGroup()` containers and set clear buffers (`buff=0.2`).
6. Use `Create()` for a shape's first appearance and `Write()` for text/MathTex's first appearance — but only the *first* time an object appears. Once on screen, evolve it with `Transform`, `ReplacementTransform`, `.animate`, or `FadeOut`/`FadeIn` for a genuine scene change (e.g., moving to a new example), not to redraw something already shown. Do NOT use deprecated `ShowCreation()`.
7. Ensure all `MathTex` strings use valid LaTeX syntax.
8. Do NOT import `os`, `sys`, `subprocess`, or execute file/system operations.
