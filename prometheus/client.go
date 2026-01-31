package prometheus

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	api v1.API
}

type Config struct {
	URL      string
	Username string
	Password string
	Timeout  time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	apiCfg := api.Config{
		Address: cfg.URL,
	}

	if cfg.Username != "" && cfg.Password != "" {
		apiCfg.RoundTripper = &basicAuthTransport{
			username: cfg.Username,
			password: cfg.Password,
		}
	}

	client, err := api.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &Client{
		api: v1.NewAPI(client),
	}, nil
}

func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.api.Runtimeinfo(ctx)
	if err != nil {
		return fmt.Errorf("prometheus health check failed: %w", err)
	}
	return nil
}

func (c *Client) GetAllMetricNames(ctx context.Context) ([]string, error) {
	names, _, err := c.api.LabelValues(ctx, "__name__", nil, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get metric names: %w", err)
	}

	result := make([]string, len(names))
	for i, n := range names {
		result[i] = string(n)
	}

	return result, nil
}

func (c *Client) GetMetricCardinality(ctx context.Context, metricName string) (int, error) {
	query := fmt.Sprintf("count(%s)", metricName)

	result, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to get cardinality for %s: %w", metricName, err)
	}

	vector, ok := result.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, nil
	}

	return int(vector[0].Value), nil
}

type LabelInfo struct {
	Name        string
	UniqueCount int
}

func (c *Client) GetMetricLabels(ctx context.Context, metricName string) ([]LabelInfo, error) {
	series, _, err := c.api.Series(ctx, []string{metricName}, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get labels for %s: %w", metricName, err)
	}

	labelValues := make(map[string]map[string]struct{})
	for _, s := range series {
		for label, value := range s {
			if label == "__name__" {
				continue
			}
			labelName := string(label)
			if _, ok := labelValues[labelName]; !ok {
				labelValues[labelName] = make(map[string]struct{})
			}
			labelValues[labelName][string(value)] = struct{}{}
		}
	}

	var labels []LabelInfo
	for name, values := range labelValues {
		labels = append(labels, LabelInfo{
			Name:        name,
			UniqueCount: len(values),
		})
	}

	return labels, nil
}

type PrometheusConfig struct {
	ScrapeInterval time.Duration
}

func (c *Client) GetConfig(ctx context.Context) (*PrometheusConfig, error) {
	cfg, err := c.api.Config(ctx)
	if err != nil {
		slog.Warn("failed to get prometheus config, using defaults", "error", err)
		return &PrometheusConfig{ScrapeInterval: 15 * time.Second}, nil
	}

	_ = cfg // Could parse YAML to extract global.scrape_interval
	return &PrometheusConfig{ScrapeInterval: 15 * time.Second}, nil
}

type basicAuthTransport struct {
	username string
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.username, t.password)
	return http.DefaultTransport.RoundTrip(req)
}
