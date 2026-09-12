Write clean, executable Python Manim (Community Edition) code for a single animation scene.

Mandatory Architecture & Style Rules:
1. Define a class named GeneratedScene inheriting from Scene: `class GeneratedScene(Scene):`.
2. Strictly use relative positioning methods (`next_to`, `align_to`, `arrange`, `to_edge`, `to_corner`).
3. NEVER use literal absolute coordinate arrays e.g. `np.array([x, y, z])` or `.move_to([x, y, z])` for placement.
4. Prevent text and shape overlaps: group related elements into `VGroup()` containers and set clear buffers (`buff=0.2`).
5. Use `Create()` for geometric shapes/arrows and `Write()` for text/MathTex. Do NOT use deprecated `ShowCreation()`.
6. Ensure all `MathTex` strings use valid LaTeX syntax.
7. Do NOT import `os`, `sys`, `subprocess`, or execute file/system operations.
