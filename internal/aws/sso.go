package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
)

// ssoProfile is the SSO login of the resolved profile, in either of its two
// forms: an sso_session profile, whose start URL and region live in an
// [sso-session] section, or a legacy profile that carries them itself.
type ssoProfile struct {
	startURL    string
	region      string // sso_region of the profile, or of its [sso-session] section
	sessionName string // set for sso_session profiles only
	accountID   string // sso_account_id
	roleName    string // sso_role_name
}

// errSSOTokenExpired says that the SSO access token is missing, unreadable,
// expired or could not be refreshed. classifyError turns it into the "SSO token
// expired" problem and its hint.
var errSSOTokenExpired = errors.New("SSO token expired")

// detectSSO reads the resolved profile from the same shared files the SDK reads
// and reports its SSO login, if it has one. Static keys in the credentials file
// do not hide it: the profile is SSO-based for discovery all the same. A profile
// that does not exist, that cannot be read or whose sso_session has no section
// is not SSO.
//
// The files are passed explicitly because the SDK's defaults are fixed when the
// program starts and ignore AWS_CONFIG_FILE.
func detectSSO(ctx context.Context, s settings) (ssoProfile, bool) {
	sc, err := config.LoadSharedConfigProfile(ctx, s.profile, func(o *config.LoadSharedConfigOptions) {
		o.ConfigFiles = filesOf(s.configFile)
		o.CredentialsFiles = filesOf(s.credentialsFile)
	})
	if err != nil {
		return ssoProfile{}, false
	}

	switch {
	case sc.SSOSession != nil:
		// The start URL and the region come from the [sso-session] section.
		return ssoProfile{
			startURL:    sc.SSOSession.SSOStartURL,
			region:      sc.SSOSession.SSORegion,
			sessionName: sc.SSOSession.Name,
			accountID:   sc.SSOAccountID,
			roleName:    sc.SSORoleName,
		}, true
	case sc.SSOStartURL != "":
		return ssoProfile{
			startURL:  sc.SSOStartURL,
			region:    sc.SSORegion,
			accountID: sc.SSOAccountID,
			roleName:  sc.SSORoleName,
		}, true
	}
	return ssoProfile{}, false
}

// legacyToken is the part of an SSO token cache file that GoWatch reads.
type legacyToken struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

// readLegacyToken reads the access token from the one cache file the SDK names
// for a legacy profile. A file that is missing, unreadable, malformed, without
// its two fields or past its expiry at now gives errSSOTokenExpired. It never
// lists the cache directory and never writes.
func readLegacyToken(path string, now time.Time) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", errSSOTokenExpired
	}
	var t legacyToken
	if err := json.Unmarshal(data, &t); err != nil || t.AccessToken == "" || t.ExpiresAt == "" {
		return "", errSSOTokenExpired
	}
	expiresAt, err := time.Parse(time.RFC3339, t.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		return "", errSSOTokenExpired
	}
	return t.AccessToken, nil
}

// bearerToken returns the access token of the SSO login of p.
//
// An sso_session profile goes through the SDK's token provider, which refreshes
// an expired token with sso-oidc:CreateToken and rewrites its own cache file, as
// the AWS CLI does. A legacy profile has no refresh: the token is read from its
// cache file as it is, and an expired one is an expired login.
//
// The cache path and, for an sso_session profile, the provider are kept in
// m.work so that the credentials of the accounts to monitor use the same ones.
// Every failure is errSSOTokenExpired, with the cause attached when the SDK gave
// one. The token itself is never stored in the state or in an error.
func (m *AWSClientManager) bearerToken(ctx context.Context, p ssoProfile) (string, error) {
	w := &m.work
	w.ssoTokenPath, w.ssoTokenProvider = "", nil

	key := p.startURL
	if p.sessionName != "" {
		key = p.sessionName
	}
	path, err := ssocreds.StandardCachedTokenFilepath(key)
	if err != nil {
		return "", errSSOTokenExpired
	}
	w.ssoTokenPath = path

	if p.sessionName == "" {
		return readLegacyToken(path, m.now())
	}

	cfg := w.cfg.Copy()
	cfg.Region = p.region
	w.ssoTokenProvider = ssocreds.NewSSOTokenProvider(m.svc.OIDC(cfg), path)

	cctx, cancel := m.callContext(ctx)
	defer cancel()
	token, err := w.ssoTokenProvider.RetrieveBearerToken(cctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errSSOTokenExpired, err)
	}
	return token.Value, nil
}
