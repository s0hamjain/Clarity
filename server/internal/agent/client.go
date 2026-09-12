// Package agent is the coordinator's HTTP client for P1's agent service — the
// only part of the system that talks to an AI model. Shapes here are API.md §3
// exactly; they are a contract, so change them only when that document changes.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Per-endpoint timeouts, API.md §7. They differ by an order of magnitude —
// /vision is one model call, /scenes/render is three render attempts with
// repair — so each call sets its own rather than sharing one client timeout.
const (
	// 45s, not 30s: the agent's own Gemini call budget is 20s + 1 retry on
	// 5xx/429 = 40s worst case (API.md §7). 30s was less than that, so a slow
	// Gemini call timed out here first and surfaced as a generic internal
	// error instead of the agent's real model_error.
	VisionTimeout       = 45 * time.Second
	ExplainTimeout      = 90 * time.Second
	ScenesRenderTimeout = 11 * time.Minute
)

// Problem categories returned by /vision.
const (
	CategoryMath      = "math"
	CategoryAlgorithm = "algorithm"
	CategoryUnknown   = "unknown"
)

type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for the agent service at baseURL (AGENT_URL).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// No client-level timeout: each call derives its own from the job's
		// context, so cancelling a job aborts its in-flight agent calls.
		http: &http.Client{},
	}
}

// --- /vision (API.md §3.1) ------------------------------------------------

type VisionRequest struct {
	ImageB64   string `json:"image_b64"`
	MediaType  string `json:"media_type"`
	Guardrails bool   `json:"guardrails"`
}

type VisionResponse struct {
	// ProblemText is verbatim, and the coordinator hashes it. Any change to how
	// it is produced must bump PromptVersion.
	ProblemText string  `json:"problem_text"`
	Category    string  `json:"category"`
	Confidence  float64 `json:"confidence"`
}

// Vision transcribes a screenshot. image is either a data URL or raw base64;
// the prefix is stripped here because P1 receives raw base64.
//
// A response of category "unknown" with empty text is a valid 200, not an
// error — the caller turns it into failed / no_problem_found.
func (c *Client) Vision(ctx context.Context, image, mediaType string, guardrails bool) (*VisionResponse, error) {
	b64, detected := StripDataURL(image)
	if detected != "" {
		mediaType = detected
	}
	if mediaType == "" {
		mediaType = "image/png"
	}

	var out VisionResponse
	err := c.do(ctx, VisionTimeout, "/vision", VisionRequest{
		ImageB64:   b64,
		MediaType:  mediaType,
		Guardrails: guardrails,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// --- /explain (API.md §3.2) -----------------------------------------------

type Scene struct {
	Index           int     `json:"index"`
	Narration       string  `json:"narration"`
	Visual          string  `json:"visual"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type Storyboard struct {
	Title  string  `json:"title"`
	Scenes []Scene `json:"scenes"`
}

type ExplainRequest struct {
	ProblemText string `json:"problem_text"`
	Category    string `json:"category"`
	UserPrompt  string `json:"user_prompt"`
	Guardrails  bool   `json:"guardrails"`
}

type ExplainResponse struct {
	Explanation string     `json:"explanation"`
	Storyboard  Storyboard `json:"storyboard"`
	Revisions   int        `json:"revisions"`
}

// Explain returns the written explanation and the storyboard. The caller must
// write the explanation to the job before doing anything else with the
// storyboard (FRD §23 rule 8).
func (c *Client) Explain(ctx context.Context, req ExplainRequest) (*ExplainResponse, error) {
	var out ExplainResponse
	if err := c.do(ctx, ExplainTimeout, "/explain", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- /scenes/render (API.md §3.3) -----------------------------------------

type SceneRenderRequest struct {
	JobID           string `json:"job_id"`
	Scene           Scene  `json:"scene"`
	StoryboardTitle string `json:"storyboard_title"`
	Category        string `json:"category"`
	Guardrails      bool   `json:"guardrails"`
	WorkDir         string `json:"work_dir"`
	// Quality is the job-level manim flag, passed through unchanged to every
	// scene (FRD §23 rule 12) — concat with -c copy depends on it.
	Quality string `json:"quality"`
}

type SceneRenderResponse struct {
	OK            bool     `json:"ok"`
	ClipPath      string   `json:"clip_path"`
	Attempts      int      `json:"attempts"`
	LintRetries   int      `json:"lint_retries"`
	SnippetsUsed  []string `json:"snippets_used"`
	SnippetID     string   `json:"snippet_id"`
	Stage         string   `json:"stage"`
	LastTraceback string   `json:"last_traceback"`
}

// ScenesRender runs the Manim Generator agent for one scene. A 200 with
// ok:false is the normal "this scene didn't work out" result, not an error —
// the caller drops that scene and keeps the job going.
func (c *Client) ScenesRender(ctx context.Context, req SceneRenderRequest) (*SceneRenderResponse, error) {
	var out SceneRenderResponse
	if err := c.do(ctx, ScenesRenderTimeout, "/scenes/render", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- plumbing -------------------------------------------------------------

// StripDataURL splits "data:image/png;base64,AAAA" into its payload and media
// type. Input that is already raw base64 is returned unchanged with an empty
// media type.
func StripDataURL(s string) (b64, mediaType string) {
	if !strings.HasPrefix(s, "data:") {
		return s, ""
	}
	comma := strings.Index(s, ",")
	if comma < 0 {
		return s, ""
	}
	header := s[len("data:"):comma]
	return s[comma+1:], strings.SplitN(header, ";", 2)[0]
}

// Error is a non-2xx from the agent service, carrying API.md §4's code so the
// caller can tell a retryable model_error from a permanent bad_request.
type Error struct {
	StatusCode int
	Code       string
	Message    string
	Endpoint   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("agent %s: %d %s: %s", e.Endpoint, e.StatusCode, e.Code, e.Message)
}

// Retryable reports whether the same call might succeed if tried again.
func (e *Error) Retryable() bool {
	switch e.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusTooManyRequests:
		return true
	}
	return false
}

// maxErrorBody caps how much of a failure response we read. A stack trace or an
// HTML error page from something that isn't the agent should not be loaded
// whole just to be logged.
const maxErrorBody = 8 << 10

func (c *Client) do(ctx context.Context, timeout time.Duration, path string, in, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("agent %s: encode request: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("agent %s: build request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if id := RequestIDFrom(ctx); id != "" {
		req.Header.Set("X-Request-Id", id)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseError(resp, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("agent %s: decode response: %w", path, err)
	}
	return nil
}

// parseError reads API.md §4's envelope. The agent service currently returns it
// wrapped by FastAPI as {"detail": {"error": {...}}} rather than flat, so both
// shapes are accepted; anything else falls back to the raw body.
func parseError(resp *http.Response, path string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

	var envelope struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Detail *struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"detail"`
	}
	_ = json.Unmarshal(raw, &envelope)

	body := envelope.Error
	if body == nil && envelope.Detail != nil {
		body = envelope.Detail.Error
	}
	if body == nil {
		return &Error{
			StatusCode: resp.StatusCode,
			Code:       "internal",
			Message:    strings.TrimSpace(string(raw)),
			Endpoint:   path,
		}
	}
	return &Error{StatusCode: resp.StatusCode, Code: body.Code, Message: body.Message, Endpoint: path}
}

type ctxKey int

const requestIDKey ctxKey = iota

// WithRequestID propagates the coordinator's request ID onto agent calls, so a
// single request can be grepped across both services' logs.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom returns the request ID carried on ctx, or "" if there is none.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
