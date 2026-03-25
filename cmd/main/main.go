package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
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

	if cfg.AppScheduler {
		runScheduler(cfg, uc)
	} else {
		runOnce(cfg, uc)
	}
}

func runOnce(cfg *config.Config, uc *usecase.Usecase) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutMinutes)*time.Minute)
	defer cancel()

	slog.Info("starting backup", "hostname", cfg.Hostname)

	if err := uc.RunBackup(ctx); err != nil {
		slog.Error("backup failed", "error", err)
		os.Exit(1)
	}

	slog.Info("backup completed successfully")
}

func runScheduler(cfg *config.Config, uc *usecase.Usecase) {
	slog.Info("scheduler started",
		"interval", cfg.AppSchedulerPlan.Duration(),
		"last_backup", cfg.AppSchedulerLastTime,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	checkInterval := time.Minute
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	checkAndRunBackup(cfg, uc)

	for {
		select {
		case <-ticker.C:
			checkAndRunBackup(cfg, uc)
		case sig := <-sigChan:
			slog.Info("received signal, shutting down", "signal", sig)
			return
		}
	}
}

func checkAndRunBackup(cfg *config.Config, uc *usecase.Usecase) {
	timeSinceLastBackup := time.Since(cfg.AppSchedulerLastTime)
	scheduledInterval := cfg.AppSchedulerPlan.Duration()

	slog.Debug("scheduler check",
		"time_since_last", timeSinceLastBackup,
		"scheduled_interval", scheduledInterval,
	)

	if timeSinceLastBackup < scheduledInterval {
		return
	}

	slog.Info("starting scheduled backup", "hostname", cfg.Hostname)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutMinutes)*time.Minute)
	defer cancel()

	err := uc.RunBackup(ctx)
	if err != nil {
		slog.Error("scheduled backup failed", "error", err)
		return
	}

	cfg.AppSchedulerLastTime = time.Now()
	if err := cfg.Save(); err != nil {
		slog.Error("failed to save config after backup", "error", err)
	} else {
		slog.Info("config updated with new last_time", "last_time", cfg.AppSchedulerLastTime)
	}
}
