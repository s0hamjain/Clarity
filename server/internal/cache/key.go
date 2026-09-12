// Package cache implements the problem fingerprint that decides whether a job
// can be served from a previous render. The hash is FRD §12, exactly.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PromptVersion is part of every cache key. Bump it in the same commit as any
// prompt change in any component (FRD §23 rule 13) — otherwise the cache keeps
// serving results produced by the old prompt.
const PromptVersion = "2026-09-12b"

// Normalize lowercases, trims, and collapses every whitespace run (including
// newlines and tabs) to a single space. Nothing else. This is the only defense
// against transcription jitter between two captures of the same problem.
func Normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Hash returns the first 16 hex chars of
// sha256(PromptVersion || normalize(problemText) || normalize(userPrompt) || guardrails).
func Hash(problemText, userPrompt string, guardrails bool) string {
	guard := "false"
	if guardrails {
		guard = "true"
	}
	sum := sha256.Sum256([]byte(
		PromptVersion + "||" +
			Normalize(problemText) + "||" +
			Normalize(userPrompt) + "||" +
			guard,
	))
	return hex.EncodeToString(sum[:])[:16]
}
