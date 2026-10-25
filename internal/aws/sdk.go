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
	// cfg.Region is the SSO region of the profile in both. These clients carry the
	// access token of the login, so no endpoint override reaches them.
	SSO(cfg awssdk.Config) ssoAPI
	OIDC(cfg awssdk.Config) ssocreds.CreateTokenAPIClient
}

// sdkServices builds the real AWS SDK clients.
type sdkServices struct{}

// STS returns the SDK's STS client for cfg.
func (sdkServices) STS(cfg awssdk.Config) stsAPI { return sts.NewFromConfig(cfg) }

// SSO and OIDC return the SDK's SSO portal client and SSO OIDC client for cfg.
// Both carry the access token of the login, so they ignore every endpoint
// override (the config's base endpoint, AWS_ENDPOINT_URL, the variables of the
// two services and the services sections of the shared config) and call AWS in
// the SSO region of the profile. The SDK resolves the override into the options
// before it applies the option functions, so clearing the base endpoint in one
// is final.
func (sdkServices) SSO(cfg awssdk.Config) ssoAPI {
	return sso.NewFromConfig(cfg, ssoAtAWS)
}

// OIDC is the token service client; the same rules apply (AWS_ENDPOINT_URL_SSO_OIDC
// is the variable of its service).
func (sdkServices) OIDC(cfg awssdk.Config) ssocreds.CreateTokenAPIClient {
	return ssooidc.NewFromConfig(cfg, oidcAtAWS)
}

// ssoAtAWS and oidcAtAWS are the options that send the calls of an SSO portal
// client and of an SSO OIDC client to AWS, whatever endpoint override the client
// was built with. They are shared by everything that builds or calls these
// clients: sdkServices, and the loader options of the SDK's own SSO providers.
func ssoAtAWS(o *sso.Options)      { o.BaseEndpoint = nil }
func oidcAtAWS(o *ssooidc.Options) { o.BaseEndpoint = nil }

// ssoClientAtAWS makes every GetRoleCredentials call of the SDK's own SSO
// credential provider go to AWS, whatever endpoint override the client was built
// with. The options of the caller are kept.
type ssoClientAtAWS struct {
	ssocreds.GetRoleCredentialsAPIClient
}

func (c ssoClientAtAWS) GetRoleCredentials(ctx context.Context, in *sso.GetRoleCredentialsInput, optFns ...func(*sso.Options)) (*sso.GetRoleCredentialsOutput, error) {
	return c.GetRoleCredentialsAPIClient.GetRoleCredentials(ctx, in, append(optFns[:len(optFns):len(optFns)], ssoAtAWS)...)
}

// configLoader loads the SDK configuration. config.LoadDefaultConfig is the real
// one; tests wrap or replace it.
type configLoader func(ctx context.Context, optFns ...func(*config.LoadOptions) error) (awssdk.Config, error)
