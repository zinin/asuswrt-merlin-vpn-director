package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	BotToken            string        `json:"bot_token"`
	AllowedUsers        []string      `json:"allowed_users"`
	LogLevel            string        `json:"log_level"`
	UpdateCheckInterval time.Duration `json:"-"` // Parsed from string
}

// rawConfig is used for JSON unmarshaling with string duration
type rawConfig struct {
	BotToken            string   `json:"bot_token"`
	AllowedUsers        []string `json:"allowed_users"`
	LogLevel            string   `json:"log_level"`
	UpdateCheckInterval string   `json:"update_check_interval"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &Config{
		BotToken:     raw.BotToken,
		AllowedUsers: raw.AllowedUsers,
		LogLevel:     raw.LogLevel,
	}

	// Parse duration if provided and not "0"
	if raw.UpdateCheckInterval != "" && raw.UpdateCheckInterval != "0" {
		d, err := time.ParseDuration(raw.UpdateCheckInterval)
		if err != nil {
			return nil, err
		}
		cfg.UpdateCheckInterval = d
	}

	return cfg, nil
}
