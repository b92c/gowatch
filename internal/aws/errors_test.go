package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/smithy-go"
)

func TestProblemTexts(t *testing.T) {
	withHint := Problem{Text: "a", Hint: "b"}
	if got := withHint.Message(); got != "a - b" {
		t.Errorf("Message() = %q, want %q", got, "a - b")
	}
	if got := (Problem{Text: "a"}).Message(); got != "a" {
		t.Errorf("Message() without a hint = %q, want %q", got, "a")
	}
	if got := withHint.StatusText(); got != "error: a - b" {
		t.Errorf("StatusText() = %q, want %q", got, "error: a - b")
	}
}

func TestClassifyError(t *testing.T) {
	apiError := func(code string) error { return &smithy.GenericAPIError{Code: code, Message: "message"} }
	imdsError := errors.New("no EC2 IMDS role found, access disabled")
	legacySSO := []awssdk.CredentialSource{awssdk.CredentialSourceProfileSSOLegacy, awssdk.CredentialSourceSSOLegacy}

	noProvider := Problem{Text: textNoProvider, Hint: hintNoProvider}
	ssoExpired := Problem{Text: textSSOExpired, Hint: hintSSOExpired}
	badKeys := Problem{Text: textBadKeys, Hint: hintBadKeys}

	type classifyCase struct {
		err           error
		want          Problem
		name          string
		op            string
		sources       []awssdk.CredentialSource
		wantThrottled bool
	}
	tests := []classifyCase{
		{
			name: "throttling code", op: opGetCallerIdentity, err: apiError("ThrottlingException"),
			want: Problem{Text: textThrottled}, wantThrottled: true,
		},
		{
			name: "throttling code inside an operation error", op: opGetCallerIdentity,
			err:  &smithy.OperationError{ServiceID: "STS", OperationName: "GetCallerIdentity", Err: apiError("Throttling")},
			want: Problem{Text: textThrottled}, wantThrottled: true,
		},
		{
			name: "profile not found", op: opLoadConfig, err: config.SharedConfigProfileNotExistError{Profile: "dev"},
			want: Problem{Text: `AWS profile "dev" not found`, Hint: hintProfileMissing},
		},
		{name: "SSO token error while resolving credentials", op: opAuth, err: &ssocreds.InvalidTokenError{}, want: ssoExpired},
		{name: "SSO token error inside another error", op: opGetCallerIdentity, err: fmt.Errorf("x: %w", &ssocreds.InvalidTokenError{}), want: ssoExpired},
		{name: "no provider, IMDS was the last source", op: opAuth, err: imdsError, sources: []awssdk.CredentialSource{awssdk.CredentialSourceIMDS}, want: noProvider},
		{name: "no provider, sources unknown", op: opAuth, err: imdsError, want: noProvider},
		{
			name: "DNS failure while resolving SSO credentials", op: opAuth,
			err:     fmt.Errorf("refresh: %w", &net.DNSError{Err: "no such host", Name: "portal.sso.us-east-1.amazonaws.com"}),
			sources: legacySSO,
			want:    Problem{Text: prefixUnreachable + "DNS lookup failed"},
		},
		{name: "unknown failure of an SSO credential source", op: opAuth, err: errors.New("weird"), sources: legacySSO, want: ssoExpired},
		{name: "timeout", op: opGetCallerIdentity, err: context.DeadlineExceeded, want: Problem{Text: prefixUnreachable + "timeout"}},
		{
			name: "network error", op: opGetCallerIdentity, err: &net.OpError{Op: "dial", Err: errors.New("refused")},
			want: Problem{Text: prefixUnreachable + "network error"},
		},
		{
			name: "unknown error", op: opGetCallerIdentity, err: errors.New("boom\nsecond line fake-secret-for-tests"),
			want: Problem{Text: "GetCallerIdentity failed: boom"},
		},
		{
			// The SDK quotes the output of a credential_process that is not JSON in
			// its error: it must never reach the text.
			name: "a failed credential_process", op: opAuth,
			err: fmt.Errorf("failed to refresh cached credentials, %w", &processcreds.ProviderError{
				Err: fmt.Errorf("parse failed of process output: %s, error: %w", "fake-secret-for-tests", io.ErrUnexpectedEOF),
			}),
			sources: []awssdk.CredentialSource{awssdk.CredentialSourceProfileProcess, awssdk.CredentialSourceProcess},
			want:    Problem{Text: textProcessFailed, Hint: hintProcessFailed},
		},
	}
	for _, code := range []string{"ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "SignatureDoesNotMatch"} {
		tests = append(tests, classifyCase{name: "STS rejects the keys: " + code, op: opGetCallerIdentity, err: apiError(code), want: badKeys})
	}
	for _, code := range []string{"AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "ForbiddenException"} {
		tests = append(tests, classifyCase{
			name: "access denied: " + code, op: opGetCallerIdentity, err: apiError(code),
			want: Problem{Text: prefixAccessDenied + opGetCallerIdentity},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, throttled := classifyError(tt.op, tt.err, tt.sources)

			if got != tt.want || throttled != tt.wantThrottled {
				t.Errorf("classifyError(%q, %v, %v) = (%+v, %t), want (%+v, %t)", tt.op, tt.err, tt.sources, got, throttled, tt.want, tt.wantThrottled)
			}
			if strings.Contains(got.Message(), "fake-secret") {
				t.Errorf("the problem %+v carries a secret that was in the error", got)
			}
		})
	}
}

// An error that no rule recognizes keeps only its first line, so a secret that
// follows it never reaches the screen.
func TestUnknownErrorKeepsOnlyItsFirstLine(t *testing.T) {
	got, _ := classifyError(opGetCallerIdentity, errors.New("boom\nsecond line fake-secret-for-tests"), nil)

	if strings.Contains(got.Text, "fake-secret") || strings.Contains(got.Message(), "second line") {
		t.Errorf("Text = %q, want only the first line of the error", got.Text)
	}
}

func TestOneLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "first line only", in: "a\nb", want: "a"},
		{name: "trimmed, with a carriage return", in: "  a  \r\nb", want: "a"},
		{name: "long text is cut", in: strings.Repeat("x", 130), want: strings.Repeat("x", 117) + "..."},
		{name: "120 runes are kept", in: strings.Repeat("x", 120), want: strings.Repeat("x", 120)},
		{name: "runes, not bytes", in: strings.Repeat("é", 130), want: strings.Repeat("é", 117) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oneLine(tt.in); got != tt.want {
				t.Errorf("oneLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
