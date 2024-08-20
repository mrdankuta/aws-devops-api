package config

import (
	"gopkg.in/yaml.v2"
	"io/os"
)

type Config struct {
	OIDC  OIDCConfig  `yaml:"oidc"`
	Slack SlackConfig `yaml:"slack"`
	Tasks []TaskConfig `yaml:"tasks"`
}

type OIDCConfig struct {
	ProviderURL string `yaml:"provider_url"`
	ClientID    string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
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

	return &config, nil
}