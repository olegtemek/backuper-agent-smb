package smb

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hirochachacha/go-smb2"
	"github.com/olegtemek/backuper-agent-smb/internal/config"
)

type Smb struct {
	cfg     *config.Config
	conn    net.Conn
	session *smb2.Session
	share   *smb2.Share
}

func New(cfg *config.Config) *Smb {
	return &Smb{
		cfg: cfg,
	}
}

func (s *Smb) Connect(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.SMB.Host, s.cfg.SMB.Port)

	slog.Debug("connecting to SMB", "addr", addr)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMB server: %w", err)
	}
	s.conn = conn

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     s.cfg.SMB.Username,
			Password: s.cfg.SMB.Password,
		},
	}

	session, err := d.Dial(s.conn)
	if err != nil {
		s.conn.Close()
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	s.session = session

	share, err := session.Mount(s.cfg.SMB.Share)
	if err != nil {
		s.session.Logoff()
		s.conn.Close()
		return fmt.Errorf("failed to mount share: %w", err)
	}
	s.share = share

	slog.Info("connected to SMB share", "share", s.cfg.SMB.Share)
	return nil
}

func (s *Smb) Close() error {
	if s.share != nil {
		s.share.Umount()
	}
	if s.session != nil {
		s.session.Logoff()
	}
	if s.conn != nil {
		s.conn.Close()
	}
	return nil
}

func (s *Smb) CreateBackupDir(ctx context.Context, hostname, jobName, timestamp string) error {
	path := filepath.Join("auto_backups", hostname, jobName, timestamp)
	slog.Debug("creating backup directory", "path", path)

	parts := strings.Split(path, string(filepath.Separator))
	current := ""

	for _, part := range parts {
		if part == "" {
			continue
		}

		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}

		if err := s.share.Mkdir(current, 0755); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("failed to create directory %s: %w", current, err)
			}
		}
	}

	return nil
}

func (s *Smb) CopyPath(ctx context.Context, hostname, jobName, timestamp, localPath string) (int64, int64, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("path not found, skipping", "path", localPath)
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to stat %s: %w", localPath, err)
	}

	baseName := filepath.Base(localPath)
	remotePath := filepath.Join("auto_backups", hostname, jobName, timestamp, baseName)

	if info.IsDir() {
		return s.copyDirectory(ctx, localPath, remotePath)
	}

	return s.copyFile(localPath, remotePath)
}

func (s *Smb) copyDirectory(ctx context.Context, localDir, remoteDir string) (int64, int64, error) {
	var totalFiles, totalBytes int64

	if err := s.share.Mkdir(remoteDir, 0755); err != nil {
		if !os.IsExist(err) {
			return 0, 0, fmt.Errorf("failed to create remote directory %s: %w", remoteDir, err)
		}
	}

	entries, err := os.ReadDir(localDir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read directory %s: %w", localDir, err)
	}

	for _, entry := range entries {
		localPath := filepath.Join(localDir, entry.Name())
		remotePath := filepath.Join(remoteDir, entry.Name())

		if entry.IsDir() {
			files, bytes, err := s.copyDirectory(ctx, localPath, remotePath)
			if err != nil {
				slog.Error("failed to copy directory", "path", localPath, "error", err)
				continue
			}
			totalFiles += files
			totalBytes += bytes
		} else {
			files, bytes, err := s.copyFile(localPath, remotePath)
			if err != nil {
				slog.Error("failed to copy file", "path", localPath, "error", err)
				continue
			}
			totalFiles += files
			totalBytes += bytes
		}
	}

	return totalFiles, totalBytes, nil
}

func (s *Smb) copyFile(localPath, remotePath string) (int64, int64, error) {
	slog.Debug("copying file", "from", localPath, "to", remotePath)

	src, err := os.Open(localPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open local file: %w", err)
	}
	defer src.Close()

	dst, err := s.share.Create(remotePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create remote file: %w", err)
	}
	defer dst.Close()

	bytes, err := io.Copy(dst, src)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to copy data: %w", err)
	}

	return 1, bytes, nil
}

func (s *Smb) RotateVersions(ctx context.Context, hostname, jobName string, maxVersions int) error {
	jobPath := filepath.Join("auto_backups", hostname, jobName)

	slog.Debug("rotating versions", "path", jobPath, "max_versions", maxVersions)

	entries, err := s.share.ReadDir(jobPath)
	if err != nil {
		return fmt.Errorf("failed to read job directory: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) <= maxVersions {
		slog.Debug("no rotation needed", "current", len(versions), "max", maxVersions)
		return nil
	}

	sort.Strings(versions)

	toDelete := versions[:len(versions)-maxVersions]
	for _, version := range toDelete {
		versionPath := filepath.Join(jobPath, version)
		slog.Info("deleting old version", "path", versionPath)

		if err := s.removeAll(versionPath); err != nil {
			slog.Error("failed to delete version", "path", versionPath, "error", err)
			continue
		}
	}

	return nil
}

func (s *Smb) removeAll(path string) error {
	entries, err := s.share.ReadDir(path)
	if err != nil {
		return s.share.Remove(path)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if err := s.removeAll(fullPath); err != nil {
				return err
			}
		} else {
			if err := s.share.Remove(fullPath); err != nil {
				return err
			}
		}
	}

	return s.share.Remove(path)
}

func (s *Smb) GetFreeSpace(ctx context.Context) (int64, error) {
	statfs, err := s.share.Statfs(".")
	if err != nil {
		slog.Debug("failed to get free space", "error", err)
		return 0, fmt.Errorf("failed to get filesystem stats: %w", err)
	}

	allocationUnitSize := int64(statfs.BlockSize()) * int64(statfs.FragmentSize())
	freeSpace := int64(statfs.AvailableBlockCount()) * allocationUnitSize

	return freeSpace, nil
}
