package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	OIDC  OIDCConfig   `yaml:"oidc"`
	Slack SlackConfig  `yaml:"slack"`
	Tasks []TaskConfig `yaml:"tasks"`
}

type OIDCConfig struct {
	ProviderURL  string `yaml:"provider_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	RedirectURL  string `yaml:"redirect_url"`
}

type SlackConfig struct {
	Token string `yaml:"token"`
}

type TaskConfig struct {
	Name         string   `yaml:"name"`
	AWSAccounts  []string `yaml:"aws_accounts"`
	Service      string   `yaml:"service"`
	Command      string   `yaml:"command"`
	Schedule     string   `yaml:"schedule"`
	SlackChannel string   `yaml:"slack_channel"`
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	fmt.Printf("Loaded configuration with %d tasks\n", len(config.Tasks))
    for i, task := range config.Tasks {
        fmt.Printf("Task %d: Name=%s, Service=%s\n", i, task.Name, task.Service)
    }

	return &config, nil
}

func Save(filename string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}
