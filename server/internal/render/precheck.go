// Package render turns Manim Python source into a finished clip inside a
// locked-down Docker sandbox. Precheck is the static safety gate that runs
// before every docker run, including the agent's repair attempts (FRD §23
// rule 14) — it never trusts generated code with a container until it has
// been read.
package render

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
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
// module names. Generated Manim code should never need anything outside
// manim + stdlib; this list is intentionally generous rather than exhaustive.
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

var (
	// Matches one `import x[, y]` or `from x import ...` per line. Uses
	// [ \t] rather than \s so the capture never crosses a newline.
	importRe = regexp.MustCompile(`(?m)^[ \t]*(?:from[ \t]+([A-Za-z_][\w.]*)[ \t]+import\b|import[ \t]+([A-Za-z_][\w.,\t ]*))`)

	// Requires the exact class the pre-check and the container both need.
	classRe = regexp.MustCompile(`class\s+GeneratedScene\s*\(\s*Scene\s*\)`)

	bannedCallRes = []struct {
		re     *regexp.Regexp
		reason string
	}{
		{regexp.MustCompile(`\bos\.system\s*\(`), "calls os.system"},
		{regexp.MustCompile(`\bsubprocess\b`), "references subprocess"},
		{regexp.MustCompile(`\b__import__\s*\(`), "calls __import__"},
		{regexp.MustCompile(`\beval\s*\(`), "calls eval"},
		{regexp.MustCompile(`\bexec\s*\(`), "calls exec"},
		{regexp.MustCompile(`\bopen\s*\([^)]*["'](?:w|a|x|w\+|a\+|x\+)["']`), "opens a file for writing"},
	}
)

// Precheck runs the static safety scan every generated source file must pass
// before a docker run: valid Python, only manim/stdlib imports, none of the
// banned calls, and a GeneratedScene(Scene) class. Returns nil when clean.
func Precheck(src string) *RenderError {
	if err := checkSyntax(src); err != nil {
		return err
	}
	if err := checkImports(src); err != nil {
		return err
	}
	if err := checkBannedCalls(src); err != nil {
		return err
	}
	if !classRe.MatchString(src) {
		return &RenderError{Stage: "precheck", Traceback: "missing required `class GeneratedScene(Scene)`"}
	}
	return nil
}

func checkSyntax(src string) *RenderError {
	cmd := exec.Command("python3", "-c", "import ast,sys; ast.parse(sys.stdin.read())")
	cmd.Stdin = strings.NewReader(src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		return &RenderError{Stage: "precheck", Traceback: "invalid Python syntax: " + lines[len(lines)-1]}
	}
	return nil
}

func checkImports(src string) *RenderError {
	for _, m := range importRe.FindAllStringSubmatch(src, -1) {
		modules := m[1]
		if modules == "" {
			modules = m[2]
		}
		for _, mod := range strings.Split(modules, ",") {
			mod = strings.TrimSpace(mod)
			if mod == "" {
				continue
			}
			// "numpy as np" -> "numpy"; "a.b.c" -> root "a".
			fields := strings.Fields(mod)
			root := strings.SplitN(fields[0], ".", 2)[0]
			if root != "manim" && !stdlibModules[root] {
				return &RenderError{
					Stage:     "precheck",
					Traceback: fmt.Sprintf("disallowed import: %q (only manim and the standard library are allowed)", root),
				}
			}
		}
	}
	return nil
}

func checkBannedCalls(src string) *RenderError {
	for _, b := range bannedCallRes {
		if b.re.MatchString(src) {
			return &RenderError{Stage: "precheck", Traceback: "banned pattern: " + b.reason}
		}
	}
	return nil
}
