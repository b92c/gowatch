package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// stsAPI is the part of the STS client the poller uses.
type stsAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// services builds the service clients from a resolved SDK config. sdkServices
// is the real one; tests replace it with fakes, so no test builds a real client
// or reaches the network.
type services interface {
	STS(cfg awssdk.Config) stsAPI
}

// sdkServices builds the real AWS SDK clients.
type sdkServices struct{}

// STS returns the SDK's STS client for cfg.
func (sdkServices) STS(cfg awssdk.Config) stsAPI { return sts.NewFromConfig(cfg) }

// configLoader loads the SDK configuration. config.LoadDefaultConfig is the real
// one; tests wrap or replace it.
type configLoader func(ctx context.Context, optFns ...func(*config.LoadOptions) error) (awssdk.Config, error)
