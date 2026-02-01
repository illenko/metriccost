package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Discovery  DiscoveryConfig  `mapstructure:"discovery"`
	Scan       ScanConfig       `mapstructure:"scan"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Server     ServerConfig     `mapstructure:"server"`
}

type PrometheusConfig struct {
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type DiscoveryConfig struct {
	ServiceLabel string `mapstructure:"service_label"`
}

type ScanConfig struct {
	Interval          time.Duration `mapstructure:"interval"`
	SampleValuesLimit int           `mapstructure:"sample_values_limit"`
}

type StorageConfig struct {
	Path          string `mapstructure:"path"`
	RetentionDays int    `mapstructure:"retention_days"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix("METRICCOST")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return defaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("prometheus.url", "http://localhost:9090")
	v.SetDefault("discovery.service_label", "app")
	v.SetDefault("scan.interval", "24h")
	v.SetDefault("scan.sample_values_limit", 10)
	v.SetDefault("storage.path", "./data/metrics-audit.db")
	v.SetDefault("storage.retention_days", 90)
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
}

func defaultConfig() *Config {
	return &Config{
		Prometheus: PrometheusConfig{
			URL: "http://localhost:9090",
		},
		Discovery: DiscoveryConfig{
			ServiceLabel: "app",
		},
		Scan: ScanConfig{
			Interval:          24 * time.Hour,
			SampleValuesLimit: 10,
		},
		Storage: StorageConfig{
			Path:          "./data/metrics-audit.db",
			RetentionDays: 90,
		},
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
	}
}

func (c *Config) Validate() error {
	if c.Prometheus.URL == "" {
		return fmt.Errorf("prometheus.url is required")
	}
	if c.Discovery.ServiceLabel == "" {
		return fmt.Errorf("discovery.service_label is required")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	return nil
}

func (c *Config) RetentionDuration() time.Duration {
	return time.Duration(c.Storage.RetentionDays) * 24 * time.Hour
}

func ConfigFileExists(path string) bool {
	if path == "" {
		path = "config.yaml"
	}
	_, err := os.Stat(path)
	return err == nil
}
