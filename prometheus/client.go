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

type MetricsClient interface {
	HealthCheck(ctx context.Context) error
	DiscoverServices(ctx context.Context, serviceLabel string) ([]ServiceInfo, error)
	GetMetricsForService(ctx context.Context, serviceLabel, serviceName string) ([]MetricInfo, error)
	GetLabelsForMetric(ctx context.Context, serviceLabel, serviceName, metricName string, sampleLimit int) ([]LabelInfo, error)
}

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
	var transport = http.DefaultTransport

	if cfg.Username != "" && cfg.Password != "" {
		transport = &basicAuthTransport{
			transport: transport,
			username:  cfg.Username,
			password:  cfg.Password,
		}
	}

	apiCfg := api.Config{
		Address:      cfg.URL,
		RoundTripper: transport,
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

type ServiceInfo struct {
	Name        string
	SeriesCount int
}

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

	sort.Slice(services, func(i, j int) bool {
		return services[i].SeriesCount > services[j].SeriesCount
	})

	return services, nil
}

type MetricInfo struct {
	Name        string
	SeriesCount int
}

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

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].SeriesCount > metrics[j].SeriesCount
	})

	return metrics, nil
}

type LabelInfo struct {
	Name         string
	UniqueValues int
	SampleValues []string
}

func (c *Client) GetLabelsForMetric(ctx context.Context, serviceLabel, serviceName, metricName string, sampleLimit int) ([]LabelInfo, error) {
	selector := fmt.Sprintf(`%s{%s="%s"}`, metricName, serviceLabel, serviceName)

	series, _, err := c.api.Series(ctx, []string{selector}, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to get labels for %s: %w", metricName, err)
	}

	labelValues := make(map[string]map[string]struct{})
	for _, s := range series {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		for label, value := range s {
			labelName := string(label)
			if labelName == "__name__" || labelName == serviceLabel {
				continue
			}
			if _, ok := labelValues[labelName]; !ok {
				labelValues[labelName] = make(map[string]struct{})
			}
			labelValues[labelName][string(value)] = struct{}{}
		}
	}

	var labels []LabelInfo
	for name, values := range labelValues {
		var samples []string
		for v := range values {
			samples = append(samples, v)
			if len(samples) >= sampleLimit {
				break
			}
		}

		sort.Strings(samples)

		labels = append(labels, LabelInfo{
			Name:         name,
			UniqueValues: len(values),
			SampleValues: samples,
		})
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].UniqueValues > labels[j].UniqueValues
	})

	return labels, nil
}

type basicAuthTransport struct {
	transport http.RoundTripper
	username  string
	password  string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.SetBasicAuth(t.username, t.password)
	return t.transport.RoundTrip(req)
}
