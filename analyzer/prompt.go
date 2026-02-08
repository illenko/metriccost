package analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
	"google.golang.org/genai"
)

func getGenaiToolDefinitions() *genai.Tool {
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "get_service_metrics",
				Description: "Get all metrics for a service in a snapshot",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"snapshot_id":  {Type: genai.TypeInteger, Description: "ID of the snapshot"},
						"service_name": {Type: genai.TypeString, Description: "Name of the service"},
					},
					Required: []string{"snapshot_id", "service_name"},
				},
			},
			{
				Name:        "get_metric_labels",
				Description: "Get all labels for a specific metric",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"snapshot_id":  {Type: genai.TypeInteger, Description: "ID of the snapshot"},
						"service_name": {Type: genai.TypeString, Description: "Name of the service"},
						"metric_name":  {Type: genai.TypeString, Description: "Name of the metric"},
					},
					Required: []string{"snapshot_id", "service_name", "metric_name"},
				},
			},
			{
				Name:        "compare_services",
				Description: "Compare a service between two snapshots to see added/removed metrics and series count changes",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"current_snapshot_id":  {Type: genai.TypeInteger, Description: "ID of the current snapshot"},
						"previous_snapshot_id": {Type: genai.TypeInteger, Description: "ID of the previous snapshot"},
						"service_name":         {Type: genai.TypeString, Description: "Name of the service"},
					},
					Required: []string{"current_snapshot_id", "previous_snapshot_id", "service_name"},
				},
			},
		},
	}
}

func (a *Analyzer) buildPrompt(ctx context.Context, current, previous *models.Snapshot) (string, error) {
	currentServices, err := a.services.List(ctx, current.ID, storage.ServiceListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list current services: %w", err)
	}

	previousServices, err := a.services.List(ctx, previous.ID, storage.ServiceListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list previous services: %w", err)
	}

	prompt := fmt.Sprintf(`You are an expert monitoring system analyzer specializing in Prometheus metrics analysis. Your goals:
1. Identify significant changes between two snapshots
2. Detect high cardinality issues and anti-patterns (IDs, UUIDs, URLs in labels)

# Available Tools

You have EXACTLY 3 tools. Do NOT attempt to call any other tools or add parameters not listed:

1. get_service_metrics(snapshot_id, service_name)
   - Returns: All metrics for the specified service in the given snapshot

2. get_metric_labels(snapshot_id, service_name, metric_name)
   - Returns: All label combinations for a specific metric

3. compare_services(current_snapshot_id, previous_snapshot_id, service_name)
   - Returns: Comparison showing added/removed metrics and series count changes
---
Current snapshot (ID: %d):
- Collected at: %s
- Total services: %d
- Total series: %d
Services in this snapshot:
%s
---
Previous snapshot (ID: %d):
- Collected at: %s
- Total services: %d
- Total series: %d
Services in previous snapshot:
%s
---
# Analysis Strategy

## Phase 1: Change Detection (2-3 tool calls)
- Use compare_services on 2-3 services with notable series count differences
- Identify new/removed services from the lists above (no tool needed)

## Phase 2: Cardinality Analysis (3-4 tool calls)
**CRITICAL**: Focus on detecting anti-patterns in the CURRENT snapshot:

For services with >1000 series OR >50 percents series growth:
1. Use get_service_metrics to identify metrics with high series counts
2. Use get_metric_labels on metrics with >100 series to examine label patterns

**Red flags to detect:**
- Label values containing UUIDs/GUIDs (patterns: 8-4-4-4-12 hex digits)
- Transaction/payment/request IDs in labels (numeric IDs >6 digits, alphanumeric codes)
- User IDs, account IDs, merchant IDs in labels
- URLs or paths with variable IDs (e.g., /api/transactions/12345/status)
- Timestamps or dates in label values
- Session tokens or correlation IDs
- Email addresses or personal identifiers

**Healthy patterns:**
- Bounded enums (status: success/failed/pending)
- Service names, environment, region, availability zone
- HTTP methods, response codes (2xx, 4xx, 5xx ranges)
- Provider names (limited set)
- Payment methods (card, wallet, bank_transfer - limited set)

## Phase 3: Stop Condition
- Never call the same tool with identical parameters twice
- Stop after 7-8 total tool calls or when you have enough data
- If a tool returns no useful insights, move to different service/metric

# Output Format

## 🚨 High Cardinality Issues (if found)
For each problematic metric:
- **Metric**: service_name.metric_name
- **Series count**: X
- **Problem**: [ID pattern in label_name: sample values]
- **Impact**: Estimated memory/storage overhead
- **Fix**: Remove label or use constant value

## 📊 Significant Changes
**Critical** (1-2 points):
- New/removed services, >50 percents series changes, new metric types

**Notable** (1-2 points):
- 20-50 percents series changes, cardinality increases

## ✅ Recommendations
Priority-ordered action items (max 3):
1. [Most urgent - usually cardinality fixes]
2. [Investigation needed]
3. [Monitoring adjustments]

Keep total analysis under 200 words. Prioritize cardinality issues over normal changes.

# Detection Heuristics

When examining label values with get_metric_labels:

**UUID/GUID patterns:**
- 32 hex chars with/without dashes: 550e8400-e29b-41d4-a716-446655440000
- Look for: [0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}

**ID patterns:**
- Long numeric sequences: transaction_id="123456789012"
- Alphanumeric codes: payment_id="PAY_abc123xyz456"
- Prefixed IDs: merchant_id="MER_12345"

**URL/Path patterns:**
- /api/users/12345/transactions
- /payments/550e8400-e29b-41d4-a716-446655440000/status

**Safe cardinality check:**
If a label has >50 unique values, it's likely unbounded and needs investigation.

# Important Constraints

- Use ONLY the snapshot IDs provided above
- Maximum %d tool calls total
- Prioritize CURRENT snapshot cardinality analysis over historical comparison
- Assume operator understands Prometheus and payment systems
- Be specific: show actual problematic label values as examples`,
		current.ID,
		current.CollectedAt.Format(time.RFC3339),
		current.TotalServices,
		current.TotalSeries,
		formatServiceList(currentServices),
		previous.ID,
		previous.CollectedAt.Format(time.RFC3339),
		previous.TotalServices,
		previous.TotalSeries,
		formatServiceList(previousServices),
		maxAgenticIterations,
	)

	return prompt, nil
}

func formatServiceList(services []models.ServiceSnapshot) string {
	if len(services) == 0 {
		return "  (no services)"
	}

	result := ""
	for _, svc := range services {
		result += fmt.Sprintf("  - %s: %d series (%d metrics)\n", svc.ServiceName, svc.TotalSeries, svc.MetricCount)
	}
	return result
}
