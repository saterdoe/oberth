package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/saterdoe/oberth/internal/api"
	"github.com/saterdoe/oberth/internal/config"
)

func serve(ctx context.Context, cfg *config.Config, apiServer *api.Server) error {
	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      240 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	requestShutdown := func() {
		select {
		case quit <- syscall.SIGTERM:
		default:
		}
	}
	apiServer.SetShutdown(requestShutdown)

	if os.Getenv("OBERTH_PARENT_PIPE") == "1" {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			requestShutdown()
		}()
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", srv.Addr)
		err := srv.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		errs <- err
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	case <-quit:
	}

	slog.Info("shutting down server gracefully")
	apiServer.BeginDrain()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("forced shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}
