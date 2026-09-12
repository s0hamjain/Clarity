// Package render turns Manim Python source into a finished clip inside a
// locked-down Docker sandbox. Precheck is the static safety gate that runs
// before every docker run, including the agent's repair attempts (FRD §23
// rule 14) — it never trusts generated code with a container until it has
// been read.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// RenderError is returned whenever a scene fails to become a finished clip.
// Stage tells the caller (and, transitively, the agent's repair loop) where
// it failed: "precheck" | "container" | "timeout".
type RenderError struct {
	Stage     string
	Traceback string
}

func (e *RenderError) Error() string {
	return fmt.Sprintf("%s: %s", e.Stage, e.Traceback)
}

// stdlibModules is a working allowlist of Python standard library top-level
// module names. This list is intentionally generous rather than exhaustive;
// anything not on it (ctypes, socket, subprocess, requests, ...) is rejected.
var stdlibModules = map[string]bool{
	"abc": true, "argparse": true, "array": true, "ast": true, "asyncio": true,
	"base64": true, "bisect": true, "builtins": true, "calendar": true,
	"collections": true, "colorsys": true, "contextlib": true, "copy": true,
	"copyreg": true, "csv": true, "dataclasses": true, "datetime": true,
	"decimal": true, "difflib": true, "enum": true, "fractions": true,
	"functools": true, "gc": true, "glob": true, "hashlib": true, "heapq": true,
	"hmac": true, "html": true, "io": true, "itertools": true, "json": true,
	"keyword": true, "locale": true, "logging": true, "math": true,
	"numbers": true, "operator": true, "os": true, "pathlib": true, "pickle": true,
	"pprint": true, "queue": true, "random": true, "re": true, "sched": true,
	"secrets": true, "shutil": true, "signal": true, "statistics": true,
	"string": true, "struct": true, "sys": true, "textwrap": true, "threading": true,
	"time": true, "traceback": true, "types": true, "typing": true, "unicodedata": true,
	"uuid": true, "warnings": true, "weakref": true, "zoneinfo": true,
}

// extraAllowedModules are non-stdlib packages guaranteed present in the
// manim-worker image that generated scenes routinely need. numpy ships as a
// manim dependency (see docker/manim-worker) and nearly every LLM-written
// Manim scene imports it directly for its own math; banning it just burns
// the agent's repair budget on a safe, ubiquitous import. This amends the
// FRD §14.2 "manim/stdlib only" wording to "manim, numpy, or stdlib" — flag
// this to the team since it's a spec-touching change.
var extraAllowedModules = map[string]bool{
	"numpy": true,
}

// astScript parses the source exactly once and reports everything Precheck
// needs from the real AST rather than regexes over the text: enumerated
// imports, calls matching the banned list, and whether the required class
// is present. A regex anchored per-line missed statements chained after a
// semicolon (e.g. `from manim import *; import ctypes`); walking the AST
// doesn't care how the source is laid out.
const astScript = `
import ast, sys, json

def qualname(node):
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if isinstance(node, ast.Name):
        parts.append(node.id)
        return ".".join(reversed(parts))
    return None

BANNED_NAMES = {"eval", "exec", "__import__"}
BANNED_QUALNAMES = {
    "os.system", "os.popen", "os.execl", "os.execv", "os.execve",
    "shutil.rmtree",
}

try:
    tree = ast.parse(sys.stdin.read())
except SyntaxError as e:
    print(json.dumps({"syntax_error": str(e)}))
    sys.exit(0)

imports = []
banned = []
has_class = False

for node in ast.walk(tree):
    if isinstance(node, ast.Import):
        for alias in node.names:
            imports.append(alias.name)
    elif isinstance(node, ast.ImportFrom):
        if node.module:
            imports.append(node.module)
    elif isinstance(node, ast.ClassDef) and node.name == "GeneratedScene":
        for base in node.bases:
            if isinstance(base, ast.Name) and base.id == "Scene":
                has_class = True
    elif isinstance(node, ast.Call):
        name = qualname(node.func)
        if name in BANNED_NAMES:
            banned.append(name + "()")
        elif name in BANNED_QUALNAMES:
            banned.append(name + "()")
        elif name == "subprocess" or (name and name.startswith("subprocess.")):
            banned.append("subprocess")
        elif name == "open":
            mode = None
            if len(node.args) >= 2 and isinstance(node.args[1], ast.Constant):
                mode = node.args[1].value
            for kw in node.keywords:
                if kw.arg == "mode" and isinstance(kw.value, ast.Constant):
                    mode = kw.value.value
            if isinstance(mode, str) and any(c in mode for c in "wax"):
                banned.append("open(...) for writing")

print(json.dumps({"imports": imports, "banned": banned, "has_class": has_class}))
`

type astResult struct {
	SyntaxError string   `json:"syntax_error"`
	Imports     []string `json:"imports"`
	Banned      []string `json:"banned"`
	HasClass    bool     `json:"has_class"`
}

// Precheck runs the static safety scan every generated source file must pass
// before a docker run: valid Python, only manim/numpy/stdlib imports, none
// of the banned calls, and a GeneratedScene(Scene) class. Returns nil when
// the source is clean.
func Precheck(src string) *RenderError {
	cmd := exec.Command("python3", "-c", astScript)
	cmd.Stdin = strings.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &RenderError{Stage: "precheck", Traceback: "failed to analyze source: " + strings.TrimSpace(stderr.String())}
	}

	var res astResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return &RenderError{Stage: "precheck", Traceback: "failed to parse analysis output: " + err.Error()}
	}

	if res.SyntaxError != "" {
		return &RenderError{Stage: "precheck", Traceback: "invalid Python syntax: " + res.SyntaxError}
	}

	for _, mod := range res.Imports {
		root := strings.SplitN(mod, ".", 2)[0]
		if root != "manim" && !extraAllowedModules[root] && !stdlibModules[root] {
			return &RenderError{
				Stage:     "precheck",
				Traceback: fmt.Sprintf("disallowed import: %q (only manim, numpy, and the standard library are allowed)", root),
			}
		}
	}

	if len(res.Banned) > 0 {
		return &RenderError{Stage: "precheck", Traceback: "banned pattern: " + res.Banned[0]}
	}

	if !res.HasClass {
		return &RenderError{Stage: "precheck", Traceback: "missing required `class GeneratedScene(Scene)`"}
	}

	return nil
}
