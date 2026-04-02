package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type ModelConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type Config struct {
	BotToken       string      `json:"bot_token"`
	AllowedGroupID int64       `json:"allowed_group_id"`
	AdminUIDs      []int64     `json:"admin_uids"`
	TriggerWord    string      `json:"trigger_word"`
	AgentFile      string      `json:"agent_file"`
	Model          ModelConfig `json:"model"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (c Config) Validate() error {
	if c.BotToken == "" {
		return errors.New("bot_token is required")
	}
	if c.AllowedGroupID == 0 {
		return errors.New("allowed_group_id is required")
	}
	if len(c.AdminUIDs) == 0 {
		return errors.New("at least one admin uid is required")
	}
	if c.TriggerWord == "" {
		return errors.New("trigger_word is required")
	}
	if c.AgentFile == "" {
		return errors.New("agent_file is required")
	}
	if c.Model.BaseURL == "" || c.Model.APIKey == "" || c.Model.Model == "" {
		return fmt.Errorf("model.base_url, model.api_key, model.model are required")
	}
	return nil
}

func (c Config) IsAdmin(uid int64) bool {
	for _, a := range c.AdminUIDs {
		if a == uid {
			return true
		}
	}
	return false
}
