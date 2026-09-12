package api

import (
	"context"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/cache"
	"github.com/s0hamjain/Clarity/server/internal/config"
)

// HealthReport is the /healthz body, API.md §2.6.
type HealthReport struct {
	OK            bool   `json:"ok"`
	Atlas         bool   `json:"atlas"`
	Docker        bool   `json:"docker"`
	S3            bool   `json:"s3"`
	Agent         bool   `json:"agent"`
	Version       string `json:"version"`
	PromptVersion string `json:"prompt_version"`
}

// Pinger is anything that can confirm Atlas is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health caches the dependency checks. `docker`, `s3` and `agent` are checked at
// boot and every 60 s, not per request (API.md §2.6).
//
// Sprint 1 scope: `atlas` is a real ping, `docker` shells out to `docker info`,
// `agent` and `s3` are HTTP reachability probes. Sprint 3 replaces the S3 probe
// with a real HeadBucket once P2's s3.go lands.
type Health struct {
	cfg   *config.Config
	atlas Pinger

	mu   sync.RWMutex
	last HealthReport
}

const healthRefreshInterval = 60 * time.Second

func NewHealth(cfg *config.Config, atlas Pinger) *Health {
	return &Health{cfg: cfg, atlas: atlas}
}

// Start runs the checks once, then refreshes them until ctx is cancelled.
func (h *Health) Start(ctx context.Context) {
	h.refresh(ctx)
	go func() {
		t := time.NewTicker(healthRefreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.refresh(ctx)
			}
		}
	}()
}

// Report returns the most recent check results.
func (h *Health) Report() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.last
}

func (h *Health) refresh(ctx context.Context) {
	r := HealthReport{
		Atlas:         h.checkAtlas(ctx),
		Docker:        checkDocker(ctx),
		S3:            reachable(ctx, h.cfg.S3Endpoint),
		Agent:         reachable(ctx, h.cfg.AgentURL+"/healthz"),
		Version:       config.Version,
		PromptVersion: cache.PromptVersion,
	}
	r.OK = r.Atlas && r.Docker && r.S3 && r.Agent

	h.mu.Lock()
	h.last = r
	h.mu.Unlock()

	slog.Debug("health refreshed", "ok", r.OK, "atlas", r.Atlas, "docker", r.Docker, "s3", r.S3, "agent", r.Agent)
}

// checkAtlas reports false when the coordinator is running on the in-memory
// fake store — there is no Atlas to be healthy.
func (h *Health) checkAtlas(ctx context.Context) bool {
	if h.atlas == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return h.atlas.Ping(ctx) == nil
}

func checkDocker(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

// reachable sends a GET and treats any HTTP response as proof the service is
// up. MinIO answers an unauthenticated GET / with 403, which still means it is
// listening.
func reachable(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}
