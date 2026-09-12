package cache

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "multi-line, tab-indented, mixed case",
			in:   "  DEF Binary_Search(arr, Target):\n\t\tlo, hi = 0,   len(arr)\r\n\n\twhile lo < hi:  ",
			want: "def binary_search(arr, target): lo, hi = 0, len(arr) while lo < hi:",
		},
		{name: "empty", in: "", want: ""},
		{name: "whitespace only", in: " \n\t ", want: ""},
		{name: "already normal", in: "x + 1", want: "x + 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Two captures of the same problem that differ only in whitespace, indentation,
// and case must land on the same cache entry.
func TestHashIgnoresWhitespaceAndCase(t *testing.T) {
	a := Hash("d/dx [x^2 sin x]", "why the cosine?", false)
	b := Hash("  D/DX\t[x^2   SIN x]  \n", "Why   the\ncosine?", false)
	if a != b {
		t.Errorf("normalization failed to converge: %q != %q", a, b)
	}
}

func TestHashIsStable(t *testing.T) {
	a := Hash("integrate x^2 dx", "", false)
	b := Hash("integrate x^2 dx", "", false)
	if a != b {
		t.Errorf("same input produced different hashes: %q != %q", a, b)
	}
}

func TestGuardrailsChangesHash(t *testing.T) {
	off := Hash("integrate x^2 dx", "show me", false)
	on := Hash("integrate x^2 dx", "show me", true)
	if off == on {
		t.Errorf("guardrails true and false produced the same hash: %q", on)
	}
}

func TestUserPromptIsPartOfTheKey(t *testing.T) {
	a := Hash("integrate x^2 dx", "", false)
	b := Hash("integrate x^2 dx", "why the constant?", false)
	if a == b {
		t.Errorf("user_prompt did not affect the hash: %q", a)
	}
}

func TestHashShape(t *testing.T) {
	h := Hash("anything", "", false)
	if len(h) != 16 {
		t.Fatalf("hash length = %d, want 16 (%q)", len(h), h)
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("hash is not lowercase hex: %q", h)
		}
	}
}

// PromptVersion is baked into the key: bumping it must invalidate every entry.
func TestPromptVersionIsInTheKey(t *testing.T) {
	if PromptVersion == "" {
		t.Fatal("PromptVersion must not be empty")
	}
	want := "7fc78840b9ea7554"
	if got := Hash("integrate x^2 dx", "", false); got != want {
		t.Errorf("golden hash changed to %q; if PromptVersion was bumped on purpose, update this constant to %q", got, got)
	}
}
