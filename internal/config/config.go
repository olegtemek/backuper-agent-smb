package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	SMB            SMBConfig `json:"smb"`
	Hostname       string    `json:"hostname"`
	MaxVersions    int       `json:"max_versions"`
	TimeoutMinutes int       `json:"timeout_minutes"`
	Debug          bool      `json:"debug"`
	Telegram       Telegram  `json:"telegram"`
	Jobs           []Job     `json:"jobs"`
}

type SMBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Share    string `json:"share"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Telegram struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

type Job struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}

func New() (*Config, error) {

	configPath := flag.String("configPath", "./config.json", "config")
	flag.Parse()

	if *configPath == "" {
		return nil, fmt.Errorf("cannot get config path, add flag \"configPath\"")
	}

	cfg := Config{}

	data, err := os.ReadFile(*configPath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	err = cfg.Validate()

	return &cfg, err
}

func (c *Config) Validate() error {
	var errs []string

	// SMB
	if c.SMB.Host == "" {
		errs = append(errs, "smb.host is required")
	}
	if c.SMB.Port <= 0 || c.SMB.Port > 65535 {
		errs = append(errs, fmt.Sprintf("smb.port must be 1–65535, got %d", c.SMB.Port))
	}
	if c.SMB.Share == "" {
		errs = append(errs, "smb.share is required")
	}
	if c.SMB.Username == "" {
		errs = append(errs, "smb.username is required")
	}
	if c.SMB.Password == "" {
		errs = append(errs, "smb.password is required")
	}

	// Hostname
	if c.Hostname == "" {
		errs = append(errs, "hostname is required")
	}

	// MaxVersions
	if c.MaxVersions < 1 {
		errs = append(errs, fmt.Sprintf("max_versions must be >= 1, got %d", c.MaxVersions))
	}

	if c.TimeoutMinutes < 1 {
		errs = append(errs, fmt.Sprintf("timeout_minutes must be >= 1, got %d", c.TimeoutMinutes))
	}

	// Telegram
	if c.Telegram.Token == "" {
		errs = append(errs, "telegram.token is required")
	}
	if c.Telegram.ChatID == "" {
		errs = append(errs, "telegram.chat_id is required")
	}

	// Jobs
	if len(c.Jobs) == 0 {
		errs = append(errs, "at least one job is required")
	}
	for i, j := range c.Jobs {
		if j.Name == "" {
			errs = append(errs, fmt.Sprintf("jobs[%d].name is required", i))
		}
		if len(j.Paths) == 0 {
			errs = append(errs, fmt.Sprintf("jobs[%d].paths must not be empty", i))
		}
		for pi, p := range j.Paths {
			if strings.TrimSpace(p) == "" {
				errs = append(errs, fmt.Sprintf("jobs[%d].paths[%d] is blank", i, pi))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
