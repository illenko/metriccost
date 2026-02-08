package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Discovery  DiscoveryConfig  `mapstructure:"discovery"`
	Scan       ScanConfig       `mapstructure:"scan"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Server     ServerConfig     `mapstructure:"server"`
	Log        LogConfig        `mapstructure:"log"`
	Gemini     GeminiConfig     `mapstructure:"gemini"`
}

type PrometheusConfig struct {
	URL      string        `mapstructure:"url"`
	Username string        `mapstructure:"username"`
	Password string        `mapstructure:"password"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

type DiscoveryConfig struct {
	ServiceLabel string `mapstructure:"service_label"`
}

type ScanConfig struct {
	Interval          time.Duration `mapstructure:"interval"`
	SampleValuesLimit int           `mapstructure:"sample_values_limit"`
	Concurrency       int           `mapstructure:"concurrency"`
}

type StorageConfig struct {
	Path          string `mapstructure:"path"`
	RetentionDays int    `mapstructure:"retention_days"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type LogConfig struct {
	Level string `mapstructure:"level"`
}

type ChatConfig struct {
	Temperature     float32 `mapstructure:"temperature"`
	MaxOutputTokens int32   `mapstructure:"max_output_tokens"`
}

type GeminiConfig struct {
	APIKey  string        `mapstructure:"api_key"`
	Model   string        `mapstructure:"model"`
	Timeout time.Duration `mapstructure:"timeout"`
	Chat    ChatConfig    `mapstructure:"chat"`
}

func Load(path string) (*Config, error) {
	v := viper.New()

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix("WDT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnvs(v)

	if err := v.ReadInConfig(); err != nil {
		slog.Warn("no config file found, using env vars and defaults", "error", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func bindEnvs(v *viper.Viper) {
	keys := []string{
		"prometheus.url",
		"prometheus.username",
		"prometheus.password",
		"prometheus.timeout",
		"discovery.service_label",
		"scan.interval",
		"scan.sample_values_limit",
		"scan.concurrency",
		"storage.path",
		"storage.retention_days",
		"server.port",
		"server.host",
		"log.level",
		"gemini.api_key",
		"gemini.model",
		"gemini.timeout",
		"gemini.chat.temperature",
		"gemini.chat.max_output_tokens",
	}
	for _, key := range keys {
		v.BindEnv(key)
	}
}

func (c *Config) applyDefaults() {
	if c.Scan.Concurrency <= 0 {
		c.Scan.Concurrency = 5
	}
	if c.Prometheus.Timeout <= 0 {
		c.Prometheus.Timeout = 30 * time.Second
	}
	if c.Gemini.Timeout <= 0 {
		c.Gemini.Timeout = 2 * time.Minute
	}
	if c.Gemini.Chat.Temperature <= 0 {
		c.Gemini.Chat.Temperature = 0.1
	}
	if c.Gemini.Chat.MaxOutputTokens <= 0 {
		c.Gemini.Chat.MaxOutputTokens = 16384
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

func (c *Config) LogLevel() slog.Level {
	switch c.Log.Level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
