package config

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var Simla *Config

func NewConfig() (*Config, error) {
	config := &Config{
		Host:     "127.0.0.1",
		Services: make(map[string]Service),
	}
	viper.AddConfigPath(".")
	viper.SetConfigName(".simla")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	if err := viper.Unmarshal(config); err != nil {
		logrus.WithError(err).Fatal("failed to unmarshal config")
	}

	return config, nil
}

func (c *Config) GetService(ctx context.Context, serviceName string) (*Service, bool) {
	if service, exists := c.Services[serviceName]; exists {
		return &service, true
	}
	return nil, false
}
