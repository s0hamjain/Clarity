// Command server is the Clarity coordinator: the Go service the desktop app
// talks to. It owns the job lifecycle, the cache and the database; it contains
// no AI logic and no rendering logic.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/s0hamjain/Clarity/server/internal/agent"
	"github.com/s0hamjain/Clarity/server/internal/api"
	"github.com/s0hamjain/Clarity/server/internal/config"
	"github.com/s0hamjain/Clarity/server/internal/jobs"
	"github.com/s0hamjain/Clarity/server/internal/render"
	"github.com/s0hamjain/Clarity/server/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bootCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	mongo, err := store.Connect(bootCtx, cfg.MongoURI, cfg.MongoDB)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mongo.Close(closeCtx)
	}()

	// TTL indexes before the first request, so no job is ever written into a
	// collection that cannot expire it (FRD §9.1–9.2).
	idxCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = store.EnsureIndexes(idxCtx, mongo)
	cancel()
	if err != nil {
		return err
	}

	jobStore, cacheStore := store.NewJobs(mongo), store.NewCache(mongo)
	slog.Info("store ready", "backend", "atlas", "db", cfg.MongoDB)

	health := api.NewHealth(cfg, mongo)
	health.Start(ctx)

	agentClient := agent.New(cfg.AgentURL)

	// P2 owns both: Render runs one scene inside the sandbox, Concat stitches
	// the surviving clips and uploads the result.
	renderFn := api.RenderFunc(render.Render)
	concatFn := jobs.ConcatFunc(render.Concat)

	worker := jobs.NewWorker(cfg, jobStore, cacheStore, agentClient, concatFn)
	srv := &http.Server{
		Addr:              "127.0.0.1:" + cfg.Port,
		Handler:           api.NewServer(cfg, jobStore, cacheStore, worker, health, renderFn).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: the SSE stream in API.md §2.4 is long-lived.
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("coordinator listening",
			"addr", srv.Addr,
			"version", config.Version,
			"agent_url", cfg.AgentURL,
			"manim_quality", cfg.ManimQuality,
			"render_concurrency", cfg.RenderConcurrency,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
