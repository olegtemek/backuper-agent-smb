package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/olegtemek/backuper-agent-smb/internal/adaptors/smb"
	"github.com/olegtemek/backuper-agent-smb/internal/adaptors/telegram"
	"github.com/olegtemek/backuper-agent-smb/internal/config"
	"github.com/olegtemek/backuper-agent-smb/internal/usecase"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	loggerDefaultLevel := slog.LevelError
	if cfg.Debug {
		loggerDefaultLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: loggerDefaultLevel,
	}))

	slog.SetDefault(logger)

	smbAdapter := smb.New(cfg)
	telegramNotifier := telegram.New(cfg)

	uc := usecase.New(smbAdapter, telegramNotifier, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutMinutes)*time.Minute)
	defer cancel()

	slog.Info("starting backup", "hostname", cfg.Hostname)

	if err := uc.RunBackup(ctx); err != nil {
		slog.Error("backup failed", "error", err)
		os.Exit(1)
	}

	slog.Info("backup completed successfully")
}
