Critique the drafted explanation and storyboard against the quality and pedagogical rules.

Validation Rubric:
1. Scene Count: Must be between 2 and 5 scenes.
2. Narration Length: Narration MUST be under 90 characters per scene.
3. Visual Focus: Scene visuals MUST describe dynamic motion, geometric transformations, function plots, or data structure movements rather than restating static algebra.
4. Relative Language: Scene visual descriptions MUST use relative positioning language (e.g., "below", "to the right of", "centered above").
5. Guardrails Enforcement (CRITICAL): If `Guardrails Active` is True, the draft MUST NOT reveal final numerical values, final simplified formula answers, or complete code solutions in either the written explanation or the scene narrations/visuals. The draft must guide the student's reasoning while withholding the final answer.
6. Continuity (CRITICAL): The scenes are beats of ONE continuous animation, not independent slides. Only
   scene 1's visual may introduce the data structure, shape, or graph from nothing (verbs like "show",
   "display", "draw", "plot"). Every scene after the first MUST describe a change to that same object —
   it moves, highlights, transforms, swaps, rotates, shades, extends — using verbs like "move", "shift",
   "highlight", "swap", "rotate", "shade in", "extend", "update". Flag any scene after the first whose
   visual re-describes the whole structure as if being seen for the first time (redundant "show"/"display"/
   "draw" of something already on screen), since that is what makes the rendered video look like it keeps
   restarting.

Output `passed: false` and list all specific issue strings if any rule is violated.
