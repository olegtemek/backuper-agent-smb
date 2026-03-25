package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == "" {
		*d = 0
		return nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid duration format: %w", err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

type Config struct {
	SMB                     SMBConfig `json:"smb"`
	Hostname                string    `json:"hostname"`
	MaxVersions             int       `json:"max_versions"`
	TimeoutMinutes          int       `json:"timeout_minutes"`
	Debug                   bool      `json:"debug"`
	Telegram                Telegram  `json:"telegram"`
	Jobs                    []Job     `json:"jobs"`
	AppScheduler            bool      `json:"app_scheduler"`
	AppSchedulerPlan        Duration  `json:"app_scheduler_plan"`
	AppSchedulerLastTimeStr string    `json:"app_scheduler_last_time"`

	AppSchedulerLastTime time.Time `json:"-"`
	configPath           string
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
	configPathFlag := flag.String("configPath", "./config.json", "config")
	flag.Parse()

	if *configPathFlag == "" {
		return nil, fmt.Errorf("cannot get config path, add flag \"configPath\"")
	}

	cfg := Config{}

	data, err := os.ReadFile(*configPathFlag)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	cfg.configPath = *configPathFlag

	if cfg.AppSchedulerLastTimeStr != "" {
		parsed, err := time.Parse(time.RFC3339, cfg.AppSchedulerLastTimeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid app_scheduler_last_time format: %w", err)
		}
		cfg.AppSchedulerLastTime = parsed
	}

	err = cfg.Validate()

	return &cfg, err
}

func (c *Config) GetPath() string {
	return c.configPath
}

func (c *Config) Save() error {
	if !c.AppSchedulerLastTime.IsZero() {
		c.AppSchedulerLastTimeStr = c.AppSchedulerLastTime.Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(c.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
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

	// Scheduler validation
	if c.AppScheduler {
		if c.AppSchedulerPlan.Duration() <= 0 {
			errs = append(errs, "app_scheduler_plan must be positive duration when app_scheduler is enabled")
		}
		if c.AppSchedulerPlan.Duration() < time.Minute {
			errs = append(errs, "app_scheduler_plan must be at least 1 minute")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
