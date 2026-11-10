// Command server is AMBA's HTTP entrypoint: the JSON/SSE API, the optional
// Telegram bot, and the optional internal metrics listener.
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

	"github.com/amba/amba/backend/internal/config"
	"github.com/amba/amba/backend/internal/httpapi"
	"github.com/amba/amba/backend/internal/models"
	"github.com/amba/amba/backend/internal/pipeline"
	"github.com/amba/amba/backend/internal/security"
	"github.com/amba/amba/backend/internal/telegram"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		// Boot errors name the variable, never its value.
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := httpapi.NewDeps(cfg)
	root := http.NewServeMux()
	root.Handle("/", httpapi.NewRouterWith(cfg, deps, nil))

	if cfg.TelegramToken != "" && cfg.TelegramMode != "off" {
		bot := telegram.New(cfg.TelegramToken, cfg.TelegramSecret, cfg.PublicAppURL,
			func(ctx context.Context, req pipeline.Request) models.Dossier {
				return httpapi.BuildDossier(deps.Key, deps.Pipeline.Run(ctx, req), req)
			})
		bot.Metrics = deps.Metrics
		bot.MaxUploadMB = cfg.MaxUploadMB
		bot.Timeout = cfg.HardTimeout
		switch cfg.TelegramMode {
		case "webhook":
			// Outside the API rate limiter on purpose: every update arrives
			// from Telegram's servers and is authenticated by the secret.
			root.Handle("/telegram/webhook", security.APISecurityHeaders(bot.WebhookHandler()))
			if cfg.TelegramWebhookURL != "" {
				go func() {
					wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
					defer cancel()
					if err := bot.SetWebhook(wctx, cfg.TelegramWebhookURL); err != nil {
						slog.Warn("telegram webhook registration failed", "err", err)
						return
					}
					slog.Info("telegram webhook registered")
				}()
			}
		case "polling":
			go bot.Poll(ctx)
		}
	}

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      max(cfg.HardTimeout, cfg.VideoTimeout) + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	var metricsSrv *http.Server
	if cfg.MetricsAddr != "" {
		metricsSrv = &http.Server{Addr: cfg.MetricsAddr, Handler: deps.Metrics.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			slog.Info("metrics listening", "addr", cfg.MetricsAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("metrics listener failed", "err", err)
			}
		}()
	}

	go func() {
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		httpapi.LogReadiness(rctx, cfg, deps)
	}()

	go func() {
		slog.Info("listening", "addr", cfg.Addr(), "env", cfg.Environment, "llm_mock", cfg.LLMMock)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown incomplete", "err", err)
	}
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}
	slog.Info("stopped")
}
