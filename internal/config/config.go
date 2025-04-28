package config

import (
	"context"
)

func (c *Config) GetService(ctx context.Context, serviceName string) (*Service, bool) {
	if service, exists := c.Services[serviceName]; exists {
		return &service, true
	}
	return nil, false
}
