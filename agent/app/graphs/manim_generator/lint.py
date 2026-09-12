import ast
from typing import Optional, Set

# Allowed stdlib modules alongside manim and numpy
ALLOWED_IMPORTS: Set[str] = {
    "manim",
    "numpy",
    "math",
    "random",
    "itertools",
    "collections",
    "typing",
    "dataclasses",
    "enum",
    "functools",
    "operator",
    "string",
    "re",
}

BANNED_FUNCTIONS: Set[str] = {
    "eval",
    "exec",
    "__import__",
    "compile",
    "getattr",
    "setattr",
    "delattr",
    "ShowCreation",  # Deprecated in Manim CE, use Create() instead
}

BANNED_CALL_PATTERNS = {
    "os.system",
    "os.popen",
    "subprocess.run",
    "subprocess.Popen",
    "subprocess.call",
    "shutil.rmtree",
}


def lint_manim_code(source: str) -> Optional[str]:
    """Statically lints generated Manim source code using Python AST.

    Returns None if valid, or a one-line error string if linting fails.
    """
    if not source or not source.strip():
        return "Source code is empty"

    try:
        tree = ast.parse(source)
    except SyntaxError as e:
        return f"SyntaxError: {e.msg} at line {e.lineno}"

    has_generated_scene_class = False

    for node in ast.walk(tree):
        # 1. Check imports
        if isinstance(node, ast.Import):
            for alias in node.names:
                root_pkg = alias.name.split(".")[0]
                if root_pkg not in ALLOWED_IMPORTS:
                    return f"banned import: '{alias.name}'. Only manim, numpy, and stdlib modules allowed."

        elif isinstance(node, ast.ImportFrom):
            if node.module:
                root_pkg = node.module.split(".")[0]
                if root_pkg not in ALLOWED_IMPORTS:
                    return f"banned import from: '{node.module}'. Only manim, numpy, and stdlib modules allowed."

        # 2. Check function calls & open() modes
        elif isinstance(node, ast.Call):
            # Check direct function calls e.g. eval(), exec(), open()
            if isinstance(node.func, ast.Name):
                func_name = node.func.id
                if func_name in BANNED_FUNCTIONS:
                    return f"banned function call: '{func_name}'"

                if func_name == "open":
                    # Inspect mode arg
                    for arg in node.args:
                        if isinstance(arg, ast.Constant) and isinstance(arg.value, str):
                            if any(w in arg.value for w in ["w", "a", "x", "+", "b"]):
                                return "banned call: open() with write/append mode"
                    for kw in node.keywords:
                        if kw.arg == "mode" and isinstance(kw.value, ast.Constant) and isinstance(kw.value.value, str):
                            if any(w in kw.value.value for w in ["w", "a", "x", "+", "b"]):
                                return "banned call: open() with write/append mode"

            # Check attribute calls e.g. os.system(), subprocess.run()
            elif isinstance(node.func, ast.Attribute):
                if isinstance(node.func.value, ast.Name):
                    full_call = f"{node.func.value.id}.{node.func.attr}"
                    if full_call in BANNED_CALL_PATTERNS:
                        return f"banned call: '{full_call}'"

        # 3. Check for class GeneratedScene(Scene)
        elif isinstance(node, ast.ClassDef):
            if node.name == "GeneratedScene":
                # Check base class inherits from Scene
                for base in node.bases:
                    if isinstance(base, ast.Name) and base.id == "Scene":
                        has_generated_scene_class = True
                    elif isinstance(base, ast.Attribute) and base.attr == "Scene":
                        has_generated_scene_class = True

    if not has_generated_scene_class:
        return "missing class definition: must define 'class GeneratedScene(Scene):'"

    return None
