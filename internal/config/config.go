package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type RateLimiterConfig struct {
	Capacity       float64       `yaml:"capacity"`
	RefillRate     float64       `yaml:"refill_rate"`
	RefillInterval time.Duration `yaml:"refill_interval"`
}

type AdaptiveConfig struct {
	LowThreshold  int64 `yaml:"low_threshold"`
	HighThreshold int64 `yaml:"high_threshold"`
}

type Config struct {
	ListenPort          string            `yaml:"listen_port"`
	Servers             []string          `yaml:"servers"`
	Algorithm           string            `yaml:"algorithm"`
	HealthCheckInterval time.Duration     `yaml:"health_check_interval"`
	RateLimiter         RateLimiterConfig `yaml:"rate_limiter"`
	DBPath              string            `yaml:"db_path"`
	Adaptive            AdaptiveConfig    `yaml:"adaptive"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
