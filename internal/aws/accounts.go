package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// discoverIdentity loads the SDK configuration, resolves the credentials and
// asks STS who they belong to, each step under its own timeout. It records the
// outcome in m.work and reports whether the pass may go on: any failure,
// throttling included, ends it, and the next discovery tries again from the
// start, so keys exported again in the meantime are picked up.
func (m *AWSClientManager) discoverIdentity(ctx context.Context) bool {
	w := &m.work

	cctx, cancel := m.callContext(ctx)
	cfg, err := m.loadConfig(cctx, m.settings.loadOptions()...)
	cancel()
	if err != nil {
		p, _ := classifyError(opLoadConfig, err, nil)
		return m.failIdentity(p)
	}
	w.cfg, w.region, w.loaded = cfg, cfg.Region, true

	if w.cfg.Credentials == nil {
		return m.failIdentity(Problem{Text: textNoProvider, Hint: hintNoProvider})
	}
	cctx, cancel = m.callContext(ctx)
	_, err = w.cfg.Credentials.Retrieve(cctx)
	cancel()
	if err != nil {
		p, _ := classifyError(opAuth, err, credentialSources(w.cfg.Credentials))
		return m.failIdentity(p)
	}

	cctx, cancel = m.callContext(ctx)
	out, err := m.svc.STS(w.cfg).GetCallerIdentity(cctx, &sts.GetCallerIdentityInput{})
	cancel()
	if err != nil {
		p, _ := classifyError(opGetCallerIdentity, err, nil)
		return m.failIdentity(p)
	}

	id := awssdk.ToString(out.Account)
	w.identity = Identity{AccountID: id, ARN: awssdk.ToString(out.Arn), UserID: awssdk.ToString(out.UserId)}
	w.conn, w.connProblem = ConnConnected, Problem{}
	w.accounts = []Account{{UpdatedAt: m.now(), ID: id, Name: id, Status: StatusMonitored, Identity: true}}
	return true
}

// failIdentity records a connection problem. The row of the identity, if the
// manager has shown one, carries the problem in its status; the other rows stay
// as they were. It always reports false: the pass stops.
func (m *AWSClientManager) failIdentity(p Problem) bool {
	w := &m.work
	w.conn, w.connProblem = ConnError, p

	now := m.now()
	for i := range w.accounts {
		if w.accounts[i].Identity {
			w.accounts[i].Status = p.StatusText()
			w.accounts[i].UpdatedAt = now
		}
	}
	return false
}

// credentialSources lists the credential sources the SDK resolved p from, or
// nil when p does not say.
func credentialSources(p awssdk.CredentialsProvider) []awssdk.CredentialSource {
	if source, ok := p.(awssdk.CredentialProviderSource); ok {
		return source.ProviderSources()
	}
	return nil
}
