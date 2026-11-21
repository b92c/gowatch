package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type CloudWatchCollector struct {
	manager *AWSClientManager
}

func NewCloudWatchCollector(mgr *AWSClientManager) *CloudWatchCollector {
	return &CloudWatchCollector{manager: mgr}
}

func (cw *CloudWatchCollector) FetchMetrics(ctx context.Context, groupName string) (map[string]float64, error) {
	if cw == nil || cw.manager == nil {
		return nil, fmt.Errorf("cloudwatch collector uninitialized")
	}

	if cw.manager.HasLiveConfig() {
		body, _ := json.Marshal(map[string]any{
			"logGroupNamePrefix": groupName,
			"limit":              1,
		})
		respBytes, _, err := cw.manager.DoSignedRequest(ctx, "logs", "POST", "/", map[string]string{
			"Content-Type": "application/x-amz-json-1.1",
			"X-Amz-Target": "Logs_20140328.DescribeLogGroups",
		}, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			LogGroups []struct {
				StoredBytes int64 `json:"storedBytes"`
			} `json:"logGroups"`
		}
		if err := json.Unmarshal(respBytes, &out); err != nil {
			return nil, err
		}
		var stored float64
		if len(out.LogGroups) > 0 {
			stored = float64(out.LogGroups[0].StoredBytes)
		}
		return map[string]float64{
			"Groups":      float64(len(out.LogGroups)),
			"StoredBytes": stored,
		}, nil
	}

	return map[string]float64{
		"Invocations": 142.0,
		"Errors":      0.0,
		"DurationAvg": 45.2,
	}, nil
}

// ListLogGroups lists CloudWatch Log Groups via DescribeLogGroups when live
// config is present, or returns a sample group otherwise.
func (cw *CloudWatchCollector) ListLogGroups(ctx context.Context) ([]AWSResource, error) {
	if cw == nil || cw.manager == nil {
		return nil, fmt.Errorf("cloudwatch collector uninitialized")
	}
	if !cw.manager.HasLiveConfig() {
		return []AWSResource{cw.CollectResource(ctx, "gowatch-logs")}, nil
	}

	respBytes, _, err := cw.manager.DoSignedRequest(ctx, "logs", "POST", "/", map[string]string{
		"Content-Type": "application/x-amz-json-1.1",
		"X-Amz-Target": "Logs_20140328.DescribeLogGroups",
	}, []byte(`{"limit":25}`))
	if err != nil {
		return nil, err
	}

	var out struct {
		LogGroups []struct {
			LogGroupName    string `json:"logGroupName"`
			ARN             string `json:"arn"`
			StoredBytes     int64  `json:"storedBytes"`
			RetentionInDays int64  `json:"retentionInDays"`
		} `json:"logGroups"`
	}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}

	now := time.Now()
	region := cw.manager.Region()
	resources := make([]AWSResource, 0, len(out.LogGroups))
	for _, g := range out.LogGroups {
		arn := g.ARN
		if arn == "" {
			arn = "arn:aws:logs:" + region + ":log-group:" + g.LogGroupName
		}
		resources = append(resources, AWSResource{
			ID:     arn,
			Type:   "CloudWatchGroup",
			Name:   g.LogGroupName,
			Region: region,
			Status: "Active",
			Metrics: map[string]float64{
				"StoredBytes": float64(g.StoredBytes),
				"Retention":   float64(g.RetentionInDays),
			},
			LastUpdated: now,
		})
	}
	return resources, nil
}

func (cw *CloudWatchCollector) CollectResource(ctx context.Context, name string) AWSResource {
	metrics, _ := cw.FetchMetrics(ctx, name)
	return AWSResource{
		ID:          "arn:aws:cloudwatch:" + cw.manager.Region() + ":log-group:" + name,
		Type:        "CloudWatchGroup",
		Name:        name,
		Region:      cw.manager.Region(),
		Status:      "Active",
		Metrics:     metrics,
		LastUpdated: time.Now(),
	}
}
