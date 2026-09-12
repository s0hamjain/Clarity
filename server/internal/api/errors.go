package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// The one error envelope, API.md §4. Never a bare string, never HTML.
//
//	{ "error": { "code": …, "message": …, "details": {…} }, "request_id": "req_…" }
type errorEnvelope struct {
	Error     errorBody `json:"error"`
	RequestID string    `json:"request_id"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error codes from API.md §4 that the coordinator can emit.
const (
	CodeBadRequest           = "bad_request"
	CodeForbidden            = "forbidden"
	CodeJobNotFound          = "job_not_found"
	CodeCacheMiss            = "cache_miss"
	CodeImageTooLarge        = "image_too_large"
	CodeUnsupportedImageType = "unsupported_image_type"
	CodeRenderBusy           = "render_busy"
	CodeQueueFull            = "queue_full"
	CodeDependencyDown       = "dependency_down"
	CodeInternal             = "internal"
)

// writeJSON writes a 2xx body with the conventional content type.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write response body", "error", err)
	}
}

// writeError writes the error envelope. Every non-2xx response goes through
// here, including 404 for unknown routes.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details map[string]any) {
	requestID := RequestIDFrom(r.Context())
	slog.Warn("request failed",
		"request_id", requestID,
		"status", status,
		"code", code,
		"message", message,
		"method", r.Method,
		"path", r.URL.Path,
	)
	writeJSON(w, status, errorEnvelope{
		Error:     errorBody{Code: code, Message: message, Details: details},
		RequestID: requestID,
	})
}
