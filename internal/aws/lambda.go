package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type LambdaCollector struct {
	manager *AWSClientManager
}

func NewLambdaCollector(mgr *AWSClientManager) *LambdaCollector {
	return &LambdaCollector{manager: mgr}
}

func (l *LambdaCollector) ListFunctions(ctx context.Context) ([]AWSResource, error) {
	if l == nil || l.manager == nil {
		return nil, fmt.Errorf("lambda collector uninitialized")
	}

	if l.manager.HasLiveConfig() {
		respBytes, _, err := l.manager.DoSignedRequest(ctx, "lambda", "GET", "/2015-03-31/functions", nil, nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			Functions []struct {
				FunctionName string `json:"FunctionName"`
				FunctionArn  string `json:"FunctionArn"`
				State        string `json:"State"`
				MemorySize   int64  `json:"MemorySize"`
				Timeout      int64  `json:"Timeout"`
			} `json:"Functions"`
		}
		if err := json.Unmarshal(respBytes, &out); err != nil {
			return nil, err
		}
		now := time.Now()
		region := l.manager.Region()
		resources := make([]AWSResource, 0, len(out.Functions))
		for _, fn := range out.Functions {
			status := fn.State
			if status == "" {
				status = "Active"
			}
			resources = append(resources, AWSResource{
				ID:     fn.FunctionArn,
				Type:   "Lambda",
				Name:   fn.FunctionName,
				Region: region,
				Status: status,
				Metrics: map[string]float64{
					"MemoryMB": float64(fn.MemorySize),
					"TimeoutS": float64(fn.Timeout),
				},
				LastUpdated: now,
			})
		}
		return resources, nil
	}

	resources := []AWSResource{
		{
			ID:          "arn:aws:lambda:" + l.manager.Region() + ":function:gowatch-auth-handler",
			Type:        "Lambda",
			Name:        "gowatch-auth-handler",
			Region:      l.manager.Region(),
			Status:      "Active",
			Metrics:     map[string]float64{"Invocations": 84, "Errors": 0, "Duration": 23.5},
			LastUpdated: time.Now(),
		},
		{
			ID:          "arn:aws:lambda:" + l.manager.Region() + ":function:gowatch-api-processor",
			Type:        "Lambda",
			Name:        "gowatch-api-processor",
			Region:      l.manager.Region(),
			Status:      "Active",
			Metrics:     map[string]float64{"Invocations": 312, "Errors": 1, "Duration": 68.1},
			LastUpdated: time.Now(),
		},
	}

	return resources, nil
}
