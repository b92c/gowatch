package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// stsAPI is the part of the STS client the poller uses.
type stsAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// ssoAPI is the part of the SSO portal client the poller uses.
type ssoAPI interface {
	ListAccounts(ctx context.Context, params *sso.ListAccountsInput, optFns ...func(*sso.Options)) (*sso.ListAccountsOutput, error)
	ListAccountRoles(ctx context.Context, params *sso.ListAccountRolesInput, optFns ...func(*sso.Options)) (*sso.ListAccountRolesOutput, error)
	GetRoleCredentials(ctx context.Context, params *sso.GetRoleCredentialsInput, optFns ...func(*sso.Options)) (*sso.GetRoleCredentialsOutput, error)
}

// services builds the service clients from a resolved SDK config. sdkServices
// is the real one; tests replace it with fakes, so no test builds a real client
// or reaches the network.
type services interface {
	STS(cfg awssdk.Config) stsAPI
	// SSO and OIDC build the clients of the SSO portal and of its token service.
	// cfg.Region is the SSO region of the profile in both.
	SSO(cfg awssdk.Config) ssoAPI
	OIDC(cfg awssdk.Config) ssocreds.CreateTokenAPIClient
}

// sdkServices builds the real AWS SDK clients.
type sdkServices struct{}

// STS returns the SDK's STS client for cfg.
func (sdkServices) STS(cfg awssdk.Config) stsAPI { return sts.NewFromConfig(cfg) }

// SSO returns the SDK's SSO portal client for cfg.
func (sdkServices) SSO(cfg awssdk.Config) ssoAPI { return sso.NewFromConfig(cfg) }

// OIDC returns the SDK's SSO OIDC client for cfg.
func (sdkServices) OIDC(cfg awssdk.Config) ssocreds.CreateTokenAPIClient {
	return ssooidc.NewFromConfig(cfg)
}

// configLoader loads the SDK configuration. config.LoadDefaultConfig is the real
// one; tests wrap or replace it.
type configLoader func(ctx context.Context, optFns ...func(*config.LoadOptions) error) (awssdk.Config, error)
