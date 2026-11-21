package aws

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"
)

type CloudFormationCollector struct {
	manager *AWSClientManager
}

func NewCloudFormationCollector(mgr *AWSClientManager) *CloudFormationCollector {
	return &CloudFormationCollector{manager: mgr}
}

func (cf *CloudFormationCollector) ListStacks(ctx context.Context) ([]AWSResource, error) {
	if cf == nil || cf.manager == nil {
		return nil, fmt.Errorf("cloudformation collector uninitialized")
	}

	if cf.manager.HasLiveConfig() {
		body := []byte("Action=DescribeStacks&Version=2010-05-15")
		respBytes, _, err := cf.manager.DoSignedRequest(ctx, "cloudformation", "POST", "/", map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}, body)
		if err != nil {
			return nil, err
		}
		var out struct {
			Stacks []struct {
				StackId     string `xml:"StackId"`
				StackName   string `xml:"StackName"`
				StackStatus string `xml:"StackStatus"`
			} `xml:"DescribeStacksResult>Stacks>member"`
		}
		if err := xml.Unmarshal(respBytes, &out); err != nil {
			return nil, err
		}
		now := time.Now()
		region := cf.manager.Region()
		resources := make([]AWSResource, 0, len(out.Stacks))
		for _, st := range out.Stacks {
			id := st.StackId
			if id == "" {
				id = "arn:aws:cloudformation:" + region + ":stack/" + st.StackName
			}
			resources = append(resources, AWSResource{
				ID:          id,
				Type:        "CloudFormationStack",
				Name:        st.StackName,
				Region:      region,
				Status:      st.StackStatus,
				LastUpdated: now,
			})
		}
		return resources, nil
	}

	resources := []AWSResource{
		{
			ID:          "arn:aws:cloudformation:" + cf.manager.Region() + ":stack/gowatch-dev-stack",
			Type:        "CloudFormationStack",
			Name:        "gowatch-dev-stack",
			Region:      cf.manager.Region(),
			Status:      "CREATE_COMPLETE",
			Metrics:     map[string]float64{"ResourceCount": 6},
			LastUpdated: time.Now(),
		},
	}

	return resources, nil
}
