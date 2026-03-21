package usecase

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/olegtemek/backuper-agent-smb/internal/config"
)

var ErrBackupFailed = errors.New("backup failed")

type Storage interface {
	Connect(ctx context.Context) error
	Close() error
	CreateBackupDir(ctx context.Context, hostname, jobName, timestamp string) error
	CopyPath(ctx context.Context, hostname, jobName, timestamp, localPath string) (files int64, bytes int64, err error)
	RotateVersions(ctx context.Context, hostname, jobName string, maxVersions int) error
	GetFreeSpace(ctx context.Context) (int64, error)
}

type Notifier interface {
	SendReport(ctx context.Context, report BackupReport) error
}

type Usecase struct {
	storage  Storage
	notifier Notifier
	cfg      *config.Config
}

type JobResult struct {
	JobName      string
	Files        int64
	Bytes        int64
	Duration     time.Duration
	Error        error
	SkippedPaths []string
}

type BackupReport struct {
	Hostname      string
	Jobs          []JobResult
	TotalDuration time.Duration
	FreeSpace     int64
	MaxVersions   int
	Success       bool
}

func New(storage Storage, notifier Notifier, cfg *config.Config) *Usecase {
	return &Usecase{
		storage:  storage,
		notifier: notifier,
		cfg:      cfg,
	}
}

func (u *Usecase) RunBackup(ctx context.Context) error {
	startTime := time.Now()

	if err := u.storage.Connect(ctx); err != nil {
		return err
	}
	defer u.storage.Close()

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	report := BackupReport{
		Hostname:    u.cfg.Hostname,
		MaxVersions: u.cfg.MaxVersions,
		Jobs:        make([]JobResult, 0, len(u.cfg.Jobs)),
		Success:     true,
	}

	for _, job := range u.cfg.Jobs {
		jobResult := u.executeJob(ctx, job, timestamp)
		report.Jobs = append(report.Jobs, jobResult)

		if jobResult.Error != nil {
			report.Success = false
		}
	}

	freeSpace, _ := u.storage.GetFreeSpace(ctx)
	report.FreeSpace = freeSpace
	report.TotalDuration = time.Since(startTime)

	if err := u.notifier.SendReport(ctx, report); err != nil {
		return err
	}

	if !report.Success {
		return ErrBackupFailed
	}

	return nil
}

func (u *Usecase) executeJob(ctx context.Context, job config.Job, timestamp string) JobResult {
	jobStartTime := time.Now()
	result := JobResult{
		JobName:      job.Name,
		SkippedPaths: make([]string, 0),
	}

	existingPaths := make([]string, 0)
	for _, path := range job.Paths {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				result.SkippedPaths = append(result.SkippedPaths, path)
				continue
			}
			result.Error = err
			result.Duration = time.Since(jobStartTime)
			return result
		}
		existingPaths = append(existingPaths, path)
	}

	if len(existingPaths) == 0 {
		result.Error = errors.New("all paths are missing")
		result.Duration = time.Since(jobStartTime)
		return result
	}

	if err := u.storage.CreateBackupDir(ctx, u.cfg.Hostname, job.Name, timestamp); err != nil {
		result.Error = err
		result.Duration = time.Since(jobStartTime)
		return result
	}

	for _, path := range existingPaths {
		files, bytes, err := u.storage.CopyPath(ctx, u.cfg.Hostname, job.Name, timestamp, path)
		if err != nil {
			result.Error = err
			result.Duration = time.Since(jobStartTime)
			return result
		}
		result.Files += files
		result.Bytes += bytes
	}

	if err := u.storage.RotateVersions(ctx, u.cfg.Hostname, job.Name, u.cfg.MaxVersions); err != nil {
		result.Error = err
		result.Duration = time.Since(jobStartTime)
		return result
	}

	result.Duration = time.Since(jobStartTime)
	return result
}
