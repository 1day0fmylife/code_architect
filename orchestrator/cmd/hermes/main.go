package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hermes-opencode-team/orchestrator/internal/config"
	"hermes-opencode-team/orchestrator/internal/httpapi"
	"hermes-opencode-team/orchestrator/internal/memory"
	"hermes-opencode-team/orchestrator/internal/telegram"
	"hermes-opencode-team/orchestrator/internal/workflow"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := memory.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		logger.Error("init database", "error", err)
		os.Exit(1)
	}

	engine, err := workflow.NewEngine(cfg, store)
	if err != nil {
		logger.Error("init workflow engine", "error", err)
		os.Exit(1)
	}

	server := httpapi.NewServer(cfg, store, engine)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot = telegram.NewBot(cfg, engine, store)
		go bot.Run(ctx)
		logger.Info("telegram integration enabled")
	}

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		logger.Error("http server stopped", "error", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "error", err)
	}
}
