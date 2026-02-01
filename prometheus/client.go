package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"sort"
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

// ServiceInfo represents a discovered service with its series count
type ServiceInfo struct {
	Name        string
	SeriesCount int
}

// DiscoverServices returns all unique service values for the configured label
// Query: count({service_label}!="") by ({service_label})
func (c *Client) DiscoverServices(ctx context.Context, serviceLabel string) ([]ServiceInfo, error) {
	query := fmt.Sprintf(`count({%s!=""}) by (%s)`, serviceLabel, serviceLabel)

	result, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	var services []ServiceInfo
	for _, sample := range vector {
		serviceName := string(sample.Metric[model.LabelName(serviceLabel)])
		if serviceName == "" {
			continue
		}
		services = append(services, ServiceInfo{
			Name:        serviceName,
			SeriesCount: int(sample.Value),
		})
	}

	// Sort by series count descending
	sort.Slice(services, func(i, j int) bool {
		return services[i].SeriesCount > services[j].SeriesCount
	})

	return services, nil
}

// MetricInfo represents a metric with its series count
type MetricInfo struct {
	Name        string
	SeriesCount int
}

// GetMetricsForService returns all metrics for a specific service
// Query: count({service_label}="X") by (__name__)
func (c *Client) GetMetricsForService(ctx context.Context, serviceLabel, serviceName string) ([]MetricInfo, error) {
	query := fmt.Sprintf(`count({%s="%s"}) by (__name__)`, serviceLabel, serviceName)

	result, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics for service %s: %w", serviceName, err)
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	var metrics []MetricInfo
	for _, sample := range vector {
		metricName := string(sample.Metric[model.LabelName("__name__")])
		if metricName == "" {
			continue
		}
		metrics = append(metrics, MetricInfo{
			Name:        metricName,
			SeriesCount: int(sample.Value),
		})
	}

	// Sort by series count descending
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].SeriesCount > metrics[j].SeriesCount
	})

	return metrics, nil
}

// LabelInfo represents a label with its unique value count and sample values
type LabelInfo struct {
	Name         string
	UniqueValues int
	SampleValues []string
}

// GetLabelsForMetric returns all label names and their cardinality for a metric within a service
func (c *Client) GetLabelsForMetric(ctx context.Context, serviceLabel, serviceName, metricName string, sampleLimit int) ([]LabelInfo, error) {
	// Get all series for this metric in this service
	selector := fmt.Sprintf(`%s{%s="%s"}`, metricName, serviceLabel, serviceName)

	series, _, err := c.api.Series(ctx, []string{selector}, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get labels for %s: %w", metricName, err)
	}

	// Collect unique values per label
	labelValues := make(map[string]map[string]struct{})
	for _, s := range series {
		for label, value := range s {
			labelName := string(label)
			// Skip internal labels
			if labelName == "__name__" || labelName == serviceLabel {
				continue
			}
			if _, ok := labelValues[labelName]; !ok {
				labelValues[labelName] = make(map[string]struct{})
			}
			labelValues[labelName][string(value)] = struct{}{}
		}
	}

	// Build result with sample values
	var labels []LabelInfo
	for name, values := range labelValues {
		// Extract sample values (up to limit)
		var samples []string
		for v := range values {
			samples = append(samples, v)
			if len(samples) >= sampleLimit {
				break
			}
		}
		// Sort samples for consistency
		sort.Strings(samples)

		labels = append(labels, LabelInfo{
			Name:         name,
			UniqueValues: len(values),
			SampleValues: samples,
		})
	}

	// Sort by unique values descending
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].UniqueValues > labels[j].UniqueValues
	})

	return labels, nil
}

type basicAuthTransport struct {
	username string
	password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.username, t.password)
	return http.DefaultTransport.RoundTrip(req)
}
