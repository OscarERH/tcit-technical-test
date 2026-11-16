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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/oscarruiz/tcit-posts/backend/internal/config"
	"github.com/oscarruiz/tcit-posts/backend/internal/drive"
	"github.com/oscarruiz/tcit-posts/backend/internal/gemini"
	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
	"github.com/oscarruiz/tcit-posts/backend/internal/store"
	"github.com/oscarruiz/tcit-posts/backend/internal/workflow"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// setupSpanner creates the schema, retrying while the emulator is still booting.
func setupSpanner(ctx context.Context, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		err := store.Setup(ctx, cfg.SpannerProject, cfg.SpannerInstance, cfg.SpannerDatabase)
		if err == nil || status.Code(errors.Unwrap(err)) != codes.Unavailable {
			return err
		}
		slog.Info("waiting for Spanner", "error", err)
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	if cfg.SpannerAutoSetup {
		if err := setupSpanner(ctx, cfg); err != nil {
			return err
		}
	}

	// The client honours SPANNER_EMULATOR_HOST, so the same code talks to the
	// emulator locally and to Cloud Spanner in production.
	client, err := spanner.NewClient(ctx, cfg.SpannerDatabasePath())
	if err != nil {
		return err
	}
	defer client.Close()

	handler := &posts.Handler{Repo: store.NewPostsRepository(client)}

	if cfg.GeminiAPIKey != "" {
		handler.Briefs = gemini.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL)
	}
	if cfg.DriveCredentialsFile != "" && cfg.DriveFolderID != "" {
		files, err := drive.NewStore(ctx, cfg.DriveCredentialsFile, cfg.DriveFolderID)
		if err != nil {
			return err
		}
		handler.Files = files
	}
	if cfg.WorkflowURL != "" {
		handler.Workflow = workflow.NewClient(cfg.WorkflowURL)
	}
	slog.Info("integrations",
		"gemini", handler.Briefs != nil, "drive", handler.Files != nil, "workflow", handler.Workflow != nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	handler.Register(mux)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("API listening", "port", cfg.Port)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
