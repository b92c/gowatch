package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// awsEnvNames are blanked by isolateAWSEnv whether or not they are set, so a
// variable the host defines later can never reach a test.
var awsEnvNames = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_PROFILE",
	"AWS_REGION",
	"AWS_DEFAULT_REGION",
	"AWS_ENDPOINT_URL",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
	"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	"AWS_EC2_METADATA_DISABLED",
}

// isolateAWSEnv keeps the host's AWS setup out of a test. Every AWS_* variable
// is blanked, HOME points to a new temporary directory, and the two shared file
// variables name <home>/.aws/config and <home>/.aws/credentials, which do not
// exist yet. It returns the temporary home.
//
// Every test that builds an AWSClientManager calls it first: without it the
// ~/.aws files of the machine running the tests would enable the integration
// and a poller would reach for real credentials.
func isolateAWSEnv(t *testing.T) (home string) {
	t.Helper()
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "AWS_") {
			t.Setenv(name, "")
		}
	}
	for _, name := range awsEnvNames {
		t.Setenv(name, "")
	}
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))
	return home
}

// writeTestFile creates path, and the directories above it, with content.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
