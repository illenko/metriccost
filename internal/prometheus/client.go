package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	username   string
	password   string
}

type Config struct {
	URL      string
	Username string
	Password string
	Timeout  time.Duration
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Client{
		baseURL:  cfg.URL,
		username: cfg.Username,
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.doRequest(ctx, "GET", "/-/healthy", nil)
	if err != nil {
		return fmt.Errorf("prometheus health check failed: %w", err)
	}
	return nil
}

func (c *Client) GetAllMetricNames(ctx context.Context) ([]string, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/v1/label/__name__/values", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric names: %w", err)
	}

	var result apiResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus returned error: %s", result.Error)
	}

	var names []string
	if err := json.Unmarshal(result.Data, &names); err != nil {
		return nil, fmt.Errorf("failed to parse metric names: %w", err)
	}

	return names, nil
}

func (c *Client) GetMetricCardinality(ctx context.Context, metricName string) (int, error) {
	query := fmt.Sprintf("count(%s)", metricName)
	params := url.Values{"query": {query}}

	resp, err := c.doRequest(ctx, "GET", "/api/v1/query?"+params.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get cardinality for %s: %w", metricName, err)
	}

	var result apiResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return 0, fmt.Errorf("prometheus returned error: %s", result.Error)
	}

	var queryResult queryResponse
	if err := json.Unmarshal(result.Data, &queryResult); err != nil {
		return 0, fmt.Errorf("failed to parse query result: %w", err)
	}

	if len(queryResult.Result) == 0 {
		return 0, nil
	}

	if len(queryResult.Result[0].Value) < 2 {
		return 0, nil
	}

	var count int
	if err := json.Unmarshal(queryResult.Result[0].Value[1], &count); err != nil {
		// Try parsing as string first (Prometheus returns numbers as strings)
		var countStr string
		if err := json.Unmarshal(queryResult.Result[0].Value[1], &countStr); err != nil {
			return 0, fmt.Errorf("failed to parse count: %w", err)
		}
		fmt.Sscanf(countStr, "%d", &count)
	}

	return count, nil
}

type LabelInfo struct {
	Name        string
	UniqueCount int
}

func (c *Client) GetMetricLabels(ctx context.Context, metricName string) ([]LabelInfo, error) {
	params := url.Values{"match[]": {metricName}}

	resp, err := c.doRequest(ctx, "GET", "/api/v1/series?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get labels for %s: %w", metricName, err)
	}

	var result apiResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus returned error: %s", result.Error)
	}

	var series []map[string]string
	if err := json.Unmarshal(result.Data, &series); err != nil {
		return nil, fmt.Errorf("failed to parse series: %w", err)
	}

	labelValues := make(map[string]map[string]struct{})
	for _, s := range series {
		for label, value := range s {
			if label == "__name__" {
				continue
			}
			if _, ok := labelValues[label]; !ok {
				labelValues[label] = make(map[string]struct{})
			}
			labelValues[label][value] = struct{}{}
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
	resp, err := c.doRequest(ctx, "GET", "/api/v1/status/config", nil)
	if err != nil {
		slog.Warn("failed to get prometheus config, using defaults", "error", err)
		return &PrometheusConfig{ScrapeInterval: 15 * time.Second}, nil
	}

	var result apiResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return &PrometheusConfig{ScrapeInterval: 15 * time.Second}, nil
	}

	// Default if we can't parse the config
	return &PrometheusConfig{ScrapeInterval: 15 * time.Second}, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	return c.doRequestWithRetry(ctx, method, path, body, 3)
}

func (c *Client) doRequestWithRetry(ctx context.Context, method, path string, body io.Reader, maxRetries int) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Debug("retrying request", "attempt", attempt+1, "backoff", backoff)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := c.doSingleRequest(ctx, method, path, body)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		slog.Debug("request failed", "attempt", attempt+1, "error", err)
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) doSingleRequest(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	fullURL := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type apiResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  string          `json:"error,omitempty"`
}

type queryResponse struct {
	ResultType string `json:"resultType"`
	Result     []struct {
		Metric map[string]string `json:"metric"`
		Value  []json.RawMessage `json:"value"`
	} `json:"result"`
}
