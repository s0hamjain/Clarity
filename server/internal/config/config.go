// Package config reads the coordinator's environment (FRD §22) once at boot.
package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Version is the coordinator's version, reported by /healthz.
const Version = "0.1.0"

type Config struct {
	Port     string
	AgentURL string

	MongoURI string
	MongoDB  string

	S3Endpoint   string
	S3AccessKey  string
	S3SecretKey  string
	RenderBucket string

	RenderConcurrency int
	RenderTimeoutSec  int

	// ManimQuality is a job-level constant passed identically to every scene
	// (FRD §23 rule 12) — concat with -c copy depends on it.
	ManimQuality string

	// Fake modes. Both default to on until Sprint 4, when they are deleted.
	// With both on the server needs no Atlas, no Docker and no API keys, which
	// is what P4 builds the desktop UI against.
	FakeAgent  bool
	FakeRender bool

	// FakeVideoURL is served as video_url when FakeRender is on. Point it at a
	// sample MP4 you have put in MinIO by hand.
	FakeVideoURL string
}

// Load reads server/.env (if present) and then the environment. Real
// environment variables take precedence over the file.
func Load() (*Config, error) {
	loadDotEnv(dotEnvPath())

	c := &Config{
		Port:              env("PORT", "8080"),
		AgentURL:          env("AGENT_URL", "http://localhost:8000"),
		MongoURI:          env("MONGODB_URI", ""),
		MongoDB:           env("MONGODB_DB", "clarity"),
		S3Endpoint:        env("S3_ENDPOINT", "http://localhost:9000"),
		S3AccessKey:       env("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:       env("S3_SECRET_KEY", "minioadmin"),
		RenderBucket:      env("RENDER_BUCKET", "clarity-renders"),
		RenderConcurrency: envInt("RENDER_CONCURRENCY", max(1, runtime.NumCPU()/2)),
		RenderTimeoutSec:  envInt("RENDER_TIMEOUT_SEC", 120),
		ManimQuality:      env("MANIM_QUALITY", "-ql"),
		FakeAgent:         envBool("FAKE_AGENT", true),
		FakeRender:        envBool("FAKE_RENDER", true),
		FakeVideoURL:      env("FAKE_VIDEO_URL", "http://localhost:9000/clarity-renders/samples/sample.mp4"),
	}

	// The in-memory store is a development convenience, so it is available to
	// any faked configuration — including "real agent, fake render", which is
	// how the pipeline is exercised against P1 before Atlas is needed. A fully
	// real run always persists, and once the flags are deleted in Sprint 4 this
	// becomes an unconditional requirement.
	if c.MongoURI == "" && !c.FakeAgent && !c.FakeRender {
		return nil, fmt.Errorf("MONGODB_URI is required when both FAKE_AGENT and FAKE_RENDER are off")
	}
	if c.ManimQuality != "-ql" && c.ManimQuality != "-qm" {
		return nil, fmt.Errorf("MANIM_QUALITY must be -ql or -qm, got %q", c.ManimQuality)
	}
	return c, nil
}

// dotEnvPath finds server/.env whether the process was started from server/
// (SETUP §9) or from the repository root.
func dotEnvPath() string {
	if p := os.Getenv("CLARITY_ENV_FILE"); p != "" {
		return p
	}
	for _, p := range []string{".env", "server/.env"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ".env"
}

// UsesAtlas reports whether the coordinator is backed by MongoDB Atlas rather
// than the in-memory fake store.
func (c *Config) UsesAtlas() bool { return c.MongoURI != "" }

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil && v > 0 {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}
