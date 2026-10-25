package aws

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// isolatedHome is the home that isolateAWSEnv made for the test that is running,
// or "" when no test isolated its environment. The tests of this package never run
// in parallel (isolateAWSEnv sets variables), so a plain variable is enough.
var isolatedHome string

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
	outer := isolatedHome
	isolatedHome = home
	t.Cleanup(func() { isolatedHome = outer })
	return home
}

// isolationProblem says why the running test is not in an isolated AWS home, or
// "" when it is: HOME has to be the home isolateAWSEnv made, and the shared config
// variable has to name its file. Helpers that write under HOME ask it first.
func isolationProblem() string {
	home := os.Getenv("HOME")
	if home == "" || home != isolatedHome || os.Getenv("AWS_CONFIG_FILE") != filepath.Join(home, ".aws", "config") {
		return "HOME is not the home that isolateAWSEnv made for this test: call isolateAWSEnv first"
	}
	return ""
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

// The helpers that write under HOME (writeSSOToken) refuse to run unless the
// test isolated its environment with isolateAWSEnv. Variables that merely look
// right do not count: a machine that exports AWS_CONFIG_FILE as
// $HOME/.aws/config would otherwise get a test token in its real cache.
func TestIsolationProblem(t *testing.T) {
	t.Run("a test that did not isolate", func(t *testing.T) {
		if isolationProblem() == "" {
			t.Error("isolationProblem() is empty without isolateAWSEnv, want a problem")
		}
	})

	t.Run("a test that isolated", func(t *testing.T) {
		isolateAWSEnv(t)

		if got := isolationProblem(); got != "" {
			t.Errorf("isolationProblem() = %q after isolateAWSEnv, want none", got)
		}
	})

	t.Run("a HOME changed after the isolation", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Setenv("HOME", t.TempDir())

		if isolationProblem() == "" {
			t.Error("isolationProblem() is empty when HOME is not the isolated home, want a problem")
		}
	})

	t.Run("a machine that exports AWS_CONFIG_FILE as its own <home>/.aws/config", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(os.Getenv("HOME"), ".aws", "config"))

		if isolationProblem() == "" {
			t.Error("isolationProblem() is empty for variables that only look isolated, want a problem")
		}
	})

	t.Run("a home made by hand", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))

		if isolationProblem() == "" {
			t.Error("isolationProblem() is empty for a home that isolateAWSEnv did not make, want a problem")
		}
	})

	t.Run("the isolation ends with its test", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) { isolateAWSEnv(t) })

		if isolationProblem() == "" {
			t.Error("isolationProblem() is empty after the test that isolated has ended, want a problem")
		}
	})

	t.Run("an inner isolation leaves the outer one", func(t *testing.T) {
		isolateAWSEnv(t)
		t.Run("inner", func(t *testing.T) { isolateAWSEnv(t) })

		if got := isolationProblem(); got != "" {
			t.Errorf("isolationProblem() = %q after an inner test isolated and ended, want the outer isolation back", got)
		}
	})
}

// A test that calls writeSSOToken without isolating its environment fails and
// writes nothing, even when its variables look isolated. The call runs in a copy
// of the test binary whose HOME is a temporary directory: the failure is what is
// being checked, so it must not fail this test, and no real home is ever near it.
func TestWriteSSOTokenFailsWithoutIsolation(t *testing.T) {
	if os.Getenv("GOWATCH_TEST_UNISOLATED_TOKEN") == "1" {
		writeSSOToken(t, "https://example.awsapps.com/start", "fake-access-token", time.Now().Add(time.Hour), false)
		return
	}

	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteSSOTokenFailsWithoutIsolation$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"GOWATCH_TEST_UNISOLATED_TOKEN=1",
		"HOME="+home,
		"AWS_CONFIG_FILE="+filepath.Join(home, ".aws", "config"),
	)
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("a test that did not isolate its environment wrote a token cache file:\n%s", out)
	}
	if !strings.Contains(string(out), "call isolateAWSEnv first") {
		t.Errorf("the failure does not say what to do:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".aws")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("writeSSOToken wrote under %s although the environment was not isolated (stat error: %v)", home, statErr)
	}
}
