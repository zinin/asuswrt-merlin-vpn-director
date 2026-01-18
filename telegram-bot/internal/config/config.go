package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	BotToken     string   `json:"bot_token"`
	AllowedUsers []string `json:"allowed_users"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
