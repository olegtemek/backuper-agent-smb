package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/olegtemek/backuper-agent-smb/internal/config"
	"github.com/olegtemek/backuper-agent-smb/internal/usecase"
)

type Telegram struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Telegram {
	return &Telegram{
		cfg: cfg,
	}
}

type sendMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func (t *Telegram) SendReport(ctx context.Context, report usecase.BackupReport) error {
	text := t.formatReport(report)

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.cfg.Telegram.Token)

	reqBody := sendMessageRequest{
		ChatID:    t.cfg.Telegram.ChatID,
		Text:      text,
		ParseMode: "Markdown",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("failed to send telegram message", "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("telegram returned non-200 status", "status", resp.StatusCode)
	} else {
		slog.Info("telegram report sent successfully")
	}

	return nil
}

func (t *Telegram) formatReport(report usecase.BackupReport) string {
	if report.Success {
		return t.formatSuccessReport(report)
	}
	return t.formatErrorReport(report)
}

func (t *Telegram) formatSuccessReport(report usecase.BackupReport) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("✅ Бекап %s завершён\n\n", report.Hostname))

	for _, job := range report.Jobs {
		sizeMB := float64(job.Bytes) / 1024 / 1024
		buf.WriteString(fmt.Sprintf("📁 %s\n", job.JobName))
		buf.WriteString(fmt.Sprintf("   Файлов: %d | Размер: %.1f MB | Время: %s\n",
			job.Files, sizeMB, formatDuration(job.Duration)))

		if len(job.SkippedPaths) > 0 {
			buf.WriteString("   ⚠️ Пропущены пути:\n")
			for _, path := range job.SkippedPaths {
				buf.WriteString(fmt.Sprintf("      • %s\n", path))
			}
		}
		buf.WriteString("\n")
	}

	buf.WriteString(fmt.Sprintf("🕐 Общее время: %s\n", formatDuration(report.TotalDuration)))

	if report.FreeSpace > 0 {
		freeSpaceGB := float64(report.FreeSpace) / 1024 / 1024 / 1024
		buf.WriteString(fmt.Sprintf("💾 Свободно на диске: %.1f GB\n", freeSpaceGB))
	}

	return buf.String()
}

func (t *Telegram) formatErrorReport(report usecase.BackupReport) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("❌ Бекап %s — ошибка\n\n", report.Hostname))

	for _, job := range report.Jobs {
		if job.Error == nil {
			sizeMB := float64(job.Bytes) / 1024 / 1024
			buf.WriteString(fmt.Sprintf("📁 %s — ✅ ОК (%d файла, %.1f MB)\n",
				job.JobName, job.Files, sizeMB))
		} else {
			buf.WriteString(fmt.Sprintf("📁 %s — ❌ Ошибка: %s\n",
				job.JobName, job.Error.Error()))
		}

		if len(job.SkippedPaths) > 0 {
			buf.WriteString("   ⚠️ Пропущены пути:\n")
			for _, path := range job.SkippedPaths {
				buf.WriteString(fmt.Sprintf("      • %s\n", path))
			}
		}
	}

	buf.WriteString(fmt.Sprintf("\n🕐 Время: %s\n", formatDuration(report.TotalDuration)))

	return buf.String()
}

func formatDuration(d interface{ Seconds() float64 }) string {
	seconds := d.Seconds()
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	minutes := int(seconds / 60)
	secs := int(seconds) % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	hours := minutes / 60
	mins := minutes % 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}
