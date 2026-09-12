package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	cases := []struct {
		in         string
		key, value string
		ok         bool
	}{
		{`PORT=8080`, "PORT", "8080", true},
		{`  PORT = 8080  `, "PORT", "8080", true},
		{`export PORT=8080`, "PORT", "8080", true},
		{`MANIM_QUALITY="-ql"`, "MANIM_QUALITY", "-ql", true},
		{`MANIM_QUALITY='-ql'`, "MANIM_QUALITY", "-ql", true},
		{`EMPTY=`, "EMPTY", "", true},
		{`# a comment`, "", "", false},
		{``, "", "", false},
		{`   `, "", "", false},
		{`no equals sign`, "", "", false},
		{`=novalue`, "", "", false},
		// A Mongo password may contain anything; only a leading # is a comment.
		{`MONGODB_URI=mongodb+srv://u:p#a$s@host/clarity`, "MONGODB_URI", "mongodb+srv://u:p#a$s@host/clarity", true},
		{`URI=mongodb+srv://u:p=w@host/clarity`, "URI", "mongodb+srv://u:p=w@host/clarity", true},
	}
	for _, tc := range cases {
		k, v, ok := parseDotEnvLine(tc.in)
		if ok != tc.ok || k != tc.key || v != tc.value {
			t.Errorf("parseDotEnvLine(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, k, v, ok, tc.key, tc.value, tc.ok)
		}
	}
}

// The real environment always wins, so `FAKE_AGENT=0 go run ./cmd/server`
// overrides the file instead of being silently ignored.
func TestRealEnvironmentBeatsTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("CLARITY_TEST_FROM_FILE=file\nCLARITY_TEST_OVERRIDDEN=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLARITY_TEST_OVERRIDDEN", "environment")

	loadDotEnv(path)
	t.Cleanup(func() { os.Unsetenv("CLARITY_TEST_FROM_FILE") })

	if got := os.Getenv("CLARITY_TEST_FROM_FILE"); got != "file" {
		t.Errorf("value only in the file = %q, want \"file\"", got)
	}
	if got := os.Getenv("CLARITY_TEST_OVERRIDDEN"); got != "environment" {
		t.Errorf("value set in both = %q, want the environment to win", got)
	}
}

// A missing file is normal: fake mode configures nothing.
func TestMissingFileIsNotAnError(t *testing.T) {
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist"))
}
