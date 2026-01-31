package grafana

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/grafana/grafana-openapi-client-go/client"
	"github.com/grafana/grafana-openapi-client-go/client/search"
)

type Client struct {
	api     *client.GrafanaHTTPAPI
	baseURL string
}

type Config struct {
	URL      string
	APIToken string
	Username string
	Password string
	Timeout  time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid grafana URL: %w", err)
	}

	scheme := parsedURL.Scheme
	if scheme == "" {
		scheme = "http"
	}

	transportCfg := client.DefaultTransportConfig().
		WithHost(parsedURL.Host).
		WithSchemes([]string{scheme})

	if cfg.APIToken != "" {
		transportCfg.APIKey = cfg.APIToken
	} else if cfg.Username != "" && cfg.Password != "" {
		transportCfg.BasicAuth = url.UserPassword(cfg.Username, cfg.Password)
	}

	api := client.NewHTTPClientWithConfig(nil, transportCfg)

	return &Client{
		api:     api,
		baseURL: cfg.URL,
	}, nil
}

func (c *Client) HealthCheck(ctx context.Context) error {
	params := search.NewSearchParams().WithContext(ctx).WithLimit(ptr(int64(1)))
	_, err := c.api.Search.Search(params)
	if err != nil {
		return fmt.Errorf("grafana health check failed: %w", err)
	}
	return nil
}

type DashboardSearchResult struct {
	UID         string
	Title       string
	FolderTitle string
	FolderUID   string
	Type        string
	Tags        []string
}

func (c *Client) ListDashboards(ctx context.Context) ([]DashboardSearchResult, error) {
	params := search.NewSearchParams().
		WithContext(ctx).
		WithType(ptr("dash-db")).
		WithLimit(ptr(int64(5000)))

	resp, err := c.api.Search.Search(params)
	if err != nil {
		return nil, fmt.Errorf("failed to list dashboards: %w", err)
	}

	var results []DashboardSearchResult
	for _, hit := range resp.GetPayload() {
		results = append(results, DashboardSearchResult{
			UID:         hit.UID,
			Title:       hit.Title,
			FolderTitle: hit.FolderTitle,
			FolderUID:   hit.FolderUID,
			Type:        string(hit.Type),
			Tags:        hit.Tags,
		})
	}

	return results, nil
}

type DashboardDetail struct {
	Dashboard DashboardJSON
	Meta      DashboardMeta
}

type DashboardMeta struct {
	Slug      string
	URL       string
	UpdatedAt time.Time
	CreatedAt time.Time
}

type DashboardJSON struct {
	UID    string
	Title  string
	Panels []Panel
}

type Panel struct {
	ID      int
	Title   string
	Type    string
	Targets []Target
	Panels  []Panel // for row panels containing nested panels
}

type Target struct {
	Expr       string
	Expression string
	RefID      string
}

func (c *Client) GetDashboard(ctx context.Context, uid string) (*DashboardDetail, error) {
	resp, err := c.api.Dashboards.GetDashboardByUID(uid)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard %s: %w", uid, err)
	}

	payload := resp.GetPayload()

	detail := &DashboardDetail{
		Meta: DashboardMeta{
			URL: payload.Meta.URL,
		},
	}

	detail.Meta.UpdatedAt = time.Time(payload.Meta.Updated)
	detail.Meta.CreatedAt = time.Time(payload.Meta.Created)

	if dashboard, ok := payload.Dashboard.(map[string]interface{}); ok {
		if uid, ok := dashboard["uid"].(string); ok {
			detail.Dashboard.UID = uid
		}
		if title, ok := dashboard["title"].(string); ok {
			detail.Dashboard.Title = title
		}
		if panels, ok := dashboard["panels"].([]interface{}); ok {
			detail.Dashboard.Panels = parsePanels(panels)
		}
	}

	return detail, nil
}

func parsePanels(raw []interface{}) []Panel {
	var panels []Panel
	for _, p := range raw {
		panel, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		parsed := Panel{}
		if id, ok := panel["id"].(float64); ok {
			parsed.ID = int(id)
		}
		if title, ok := panel["title"].(string); ok {
			parsed.Title = title
		}
		if typ, ok := panel["type"].(string); ok {
			parsed.Type = typ
		}

		if targets, ok := panel["targets"].([]interface{}); ok {
			for _, t := range targets {
				target, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				parsedTarget := Target{}
				if expr, ok := target["expr"].(string); ok {
					parsedTarget.Expr = expr
				}
				if expr, ok := target["expression"].(string); ok {
					parsedTarget.Expression = expr
				}
				if refID, ok := target["refId"].(string); ok {
					parsedTarget.RefID = refID
				}
				parsed.Targets = append(parsed.Targets, parsedTarget)
			}
		}

		if nestedPanels, ok := panel["panels"].([]interface{}); ok {
			parsed.Panels = parsePanels(nestedPanels)
		}

		panels = append(panels, parsed)
	}
	return panels
}

func ptr[T any](v T) *T {
	return &v
}
