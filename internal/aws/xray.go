package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type XRayCollector struct {
	manager *AWSClientManager
}

func NewXRayCollector(mgr *AWSClientManager) *XRayCollector {
	return &XRayCollector{manager: mgr}
}

func (x *XRayCollector) FetchServiceGraph(ctx context.Context) ([]AWSResource, error) {
	if x == nil || x.manager == nil {
		return nil, fmt.Errorf("xray collector uninitialized")
	}

	if x.manager.HasLiveConfig() {
		now := time.Now().UTC()
		body, _ := json.Marshal(map[string]any{
			"StartTime": now.Add(-15 * time.Minute).Unix(),
			"EndTime":   now.Unix(),
		})
		respBytes, _, err := x.manager.DoSignedRequest(ctx, "xray", "POST", "/ServiceGraph", map[string]string{
			"Content-Type": "application/json",
		}, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			Services []struct {
				Name              string `json:"Name"`
				Type              string `json:"Type"`
				SummaryStatistics struct {
					OkCount           int64                      `json:"OkCount"`
					ErrorStatistics   struct{ TotalCount int64 } `json:"ErrorStatistics"`
					TotalResponseTime float64                    `json:"TotalResponseTime"`
					TotalCount        int64                      `json:"TotalCount"`
				} `json:"SummaryStatistics"`
			} `json:"Services"`
		}
		if err := json.Unmarshal(respBytes, &out); err != nil {
			return nil, err
		}
		region := x.manager.Region()
		resources := make([]AWSResource, 0, len(out.Services))
		for _, svc := range out.Services {
			avgMs := 0.0
			if svc.SummaryStatistics.TotalCount > 0 {
				avgMs = (svc.SummaryStatistics.TotalResponseTime / float64(svc.SummaryStatistics.TotalCount)) * 1000.0
			}
			status := "OK"
			if svc.SummaryStatistics.ErrorStatistics.TotalCount > 0 {
				status = "WARN"
			}
			resources = append(resources, AWSResource{
				ID:     "arn:aws:xray:" + region + ":service:" + svc.Name,
				Type:   "XRayService",
				Name:   svc.Name,
				Region: region,
				Status: status,
				Metrics: map[string]float64{
					"LatencyMs":  avgMs,
					"OkCount":    float64(svc.SummaryStatistics.OkCount),
					"ErrorCount": float64(svc.SummaryStatistics.ErrorStatistics.TotalCount),
				},
				LastUpdated: now,
			})
		}
		return resources, nil
	}

	resources := []AWSResource{
		{
			ID:          "arn:aws:xray:" + x.manager.Region() + ":service:api-gateway",
			Type:        "XRayService",
			Name:        "api-gateway",
			Region:      x.manager.Region(),
			Status:      "OK",
			Metrics:     map[string]float64{"LatencyMs": 18.4, "OkCount": 540, "ErrorCount": 0},
			LastUpdated: time.Now(),
		},
	}

	return resources, nil
}
