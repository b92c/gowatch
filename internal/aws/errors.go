package aws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/smithy-go"
)

// Problem is what the dashboard shows when an AWS step fails: a short text and,
// when one is known, what the user can do about it.
type Problem struct {
	Text string // short, user-facing
	Hint string // what to do; empty when unknown
}

// Message is the text, followed by " - " and the hint when there is one.
func (p Problem) Message() string {
	if p.Hint == "" {
		return p.Text
	}
	return p.Text + " - " + p.Hint
}

// StatusText is the message as a status: "error: " and then the message.
func (p Problem) StatusText() string {
	return "error: " + p.Message()
}

// The steps classifyError is told about. They are also the words of the
// generic message "<step> failed: ...". The last four are the SSO steps: getting
// the access token of the login and the three calls made with it.
const (
	opLoadConfig        = "AWS config"
	opAuth              = "AWS credentials"
	opGetCallerIdentity = "GetCallerIdentity"
	opSSOAccess         = "SSO token"
	opListAccounts      = "ListAccounts"
	opListAccountRoles  = "ListAccountRoles"
	opRoleAccess        = "GetRoleCredentials"
)

// The fixed texts of the problems the dashboard knows how to explain.
const (
	textNoProvider     = "no AWS credentials found"
	hintNoProvider     = "configure the AWS CLI or set aws.enabled: false"
	textSSOExpired     = "SSO token expired"
	hintSSOExpired     = "run aws sso login"
	textBadKeys        = "AWS credentials expired or invalid"
	hintBadKeys        = "log in again (for SSO logins, aws sso login and re-export)"
	textThrottled      = "throttled by AWS, slowing down"
	prefixAccessDenied = "access denied: "
	prefixUnreachable  = "AWS unreachable: "
	hintProfileMissing = "check AWS_PROFILE or aws.profile"
	textProcessFailed  = "credential_process failed"
	hintProcessFailed  = "check the credential_process of the profile"
)

// maxProblemRunes is the longest one-line error text; longer ones are cut.
const maxProblemRunes = 120

// codeUnauthorized is what the SSO portal answers when it does not accept the
// access token.
const codeUnauthorized = "UnauthorizedException"

// API error codes that mean the credentials were rejected, and that mean the
// caller may not do what it asked.
var (
	badKeyCodes = map[string]struct{}{
		"ExpiredToken":                {},
		"ExpiredTokenException":       {},
		"InvalidClientTokenId":        {},
		"UnrecognizedClientException": {},
		"SignatureDoesNotMatch":       {},
	}
	accessDeniedCodes = map[string]struct{}{
		"AccessDenied":          {},
		"AccessDeniedException": {},
		"UnauthorizedOperation": {},
		"ForbiddenException":    {},
	}
)

// ssoSources are the credential sources that need a valid SSO login.
var ssoSources = []awssdk.CredentialSource{
	awssdk.CredentialSourceSSO,
	awssdk.CredentialSourceSSOLegacy,
	awssdk.CredentialSourceProfileSSO,
	awssdk.CredentialSourceProfileSSOLegacy,
}

// classifyError maps the error of one step (op) to what the dashboard shows.
// sources are the credential sources the SDK resolved (nil when unknown).
// throttled reports a throttling error: the caller then skips the rest of the
// pass.
//
// The rules apply in this order and the first match wins:
//  1. a throttling API code;
//  2. a profile that does not exist;
//  3. an invalid SSO token anywhere in the chain, then, while resolving the
//     credentials, no known source or IMDS as the last one: no provider found;
//  4. a timeout, a DNS failure or another network error;
//  5. an SSO access token that is missing, expired or could not be refreshed
//     (errSSOTokenExpired);
//  6. while resolving the credentials, an SSO source;
//  7. on an SSO step, the portal answering that it does not accept the token
//     (UnauthorizedException);
//  8. an API code that says the keys were rejected;
//  9. an API code that says access is denied;
//  10. a credential_process that failed;
//  11. anything else, cut to the first line of the error.
//
// Rule 10 never quotes the error: when the output of the process is not valid
// JSON the SDK puts it in the error, and the process prints the credential.
func classifyError(op string, err error, sources []awssdk.CredentialSource) (p Problem, throttled bool) {
	var apiErr smithy.APIError
	hasAPIErr := errors.As(err, &apiErr)

	if hasAPIErr {
		if _, ok := retry.DefaultThrottleErrorCodes[apiErr.ErrorCode()]; ok {
			return Problem{Text: textThrottled}, true
		}
	}

	var profileErr config.SharedConfigProfileNotExistError
	if errors.As(err, &profileErr) {
		return Problem{Text: fmt.Sprintf("AWS profile %q not found", profileErr.Profile), Hint: hintProfileMissing}, false
	}

	var tokenErr *ssocreds.InvalidTokenError
	if errors.As(err, &tokenErr) {
		return Problem{Text: textSSOExpired, Hint: hintSSOExpired}, false
	}
	if op == opAuth && (len(sources) == 0 || sources[len(sources)-1] == awssdk.CredentialSourceIMDS) {
		return Problem{Text: textNoProvider, Hint: hintNoProvider}, false
	}

	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Problem{Text: prefixUnreachable + "timeout"}, false
	case errors.As(err, &dnsErr):
		return Problem{Text: prefixUnreachable + "DNS lookup failed"}, false
	case errors.As(err, &netErr):
		return Problem{Text: prefixUnreachable + "network error"}, false
	}

	if errors.Is(err, errSSOTokenExpired) {
		return Problem{Text: textSSOExpired, Hint: hintSSOExpired}, false
	}

	if op == opAuth && hasSSOSource(sources) {
		return Problem{Text: textSSOExpired, Hint: hintSSOExpired}, false
	}

	if hasAPIErr {
		code := apiErr.ErrorCode()
		if code == codeUnauthorized && isSSOStep(op) {
			return Problem{Text: textSSOExpired, Hint: hintSSOExpired}, false
		}
		if _, ok := badKeyCodes[code]; ok {
			return Problem{Text: textBadKeys, Hint: hintBadKeys}, false
		}
		if _, ok := accessDeniedCodes[code]; ok {
			return Problem{Text: prefixAccessDenied + op}, false
		}
	}

	var processErr *processcreds.ProviderError
	if errors.As(err, &processErr) {
		return Problem{Text: textProcessFailed, Hint: hintProcessFailed}, false
	}

	return Problem{Text: op + " failed: " + oneLine(err.Error())}, false
}

// isSSOStep reports whether op is a step that uses the SSO access token.
func isSSOStep(op string) bool {
	switch op {
	case opSSOAccess, opListAccounts, opListAccountRoles, opRoleAccess:
		return true
	}
	return false
}

// hasSSOSource reports whether any of the sources is an SSO login.
func hasSSOSource(sources []awssdk.CredentialSource) bool {
	for _, source := range sources {
		for _, sso := range ssoSources {
			if source == sso {
				return true
			}
		}
	}
	return false
}

// oneLine keeps the first line of s, trimmed, and cuts it to 117 runes plus
// "..." when it is longer than 120. SDK errors can span several lines and carry
// details, such as request bodies, that never belong on the screen.
func oneLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	line = strings.TrimSpace(line)
	if runes := []rune(line); len(runes) > maxProblemRunes {
		return string(runes[:maxProblemRunes-3]) + "..."
	}
	return line
}
