package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// apiErr fakes an AWS SDK error, which is identified by an ErrorCode method
// rather than a sentinel value.
type apiErr struct{ code string }

func (e *apiErr) Error() string     { return "operation error Service: Op, " + e.code }
func (e *apiErr) ErrorCode() string { return e.code }

// sdkNoise is a realistic SDK credential-chain failure. It is the exact class
// of message that must never reach a user: every word is true and none of it
// tells you what to do.
var sdkNoise = errors.New(
	"operation error STS: GetCallerIdentity, get identity: get credentials: " +
		"failed to refresh cached credentials, no EC2 IMDS role found, " +
		"operation error ec2imds: GetMetadata, access disabled to EC2 IMDS via " +
		`client option, or "AWS_EC2_METADATA_DISABLED" environment variable`)

func envWithSSO(t *testing.T, session string, expiry time.Time, hasToken bool) TestEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg,
		[]byte("[sso-session "+session+"]\nsso_start_url = https://x\n\n[profile dev]\nsso_session = "+session+"\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "sso")
	if hasToken {
		if err := os.MkdirAll(cache, 0o700); err != nil {
			t.Fatal(err)
		}
		tok := `{"sessionName":"` + session + `","expiresAt":"` + expiry.UTC().Format(time.RFC3339) + `"}`
		if err := os.WriteFile(filepath.Join(cache, "t.json"), []byte(tok), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return TestEnv{
		Vars: map[string]string{}, ConfigPath: cfg,
		CredsPath: filepath.Join(dir, "credentials"), SSOCachePth: cache,
	}
}

func asCoreError(t *testing.T, err error) *Error {
	t.Helper()
	var fe *Error
	if !errors.As(err, &fe) {
		t.Fatalf("error is not a *core.Error: %v", err)
	}
	return fe
}

func TestExplainExpiredSSOSession(t *testing.T) {
	env := envWithSSO(t, "acme-sso", time.Now().Add(-42*time.Minute), true)
	fe := asCoreError(t, NewResolver(env).explainCredentialFailure(sdkNoise))

	if fe.Code != CodeAuthExpired {
		t.Errorf("code = %q, want %q", fe.Code, CodeAuthExpired)
	}
	if !strings.Contains(fe.Message, "acme-sso") {
		t.Errorf("message should name the session, got %q", fe.Message)
	}
	if fe.Remediation != "aws sso login --sso-session acme-sso" {
		t.Errorf("remediation = %q, want the exact login command", fe.Remediation)
	}
	if fe.Exit != 3 {
		t.Errorf("exit = %d, want 3", fe.Exit)
	}
}

// Regression: configured-but-never-signed-in was classified as a generic AWS
// error, so the picker never opened — the single most likely first-run state
// for anyone using IAM Identity Center.
func TestExplainNotSignedInOpensThePicker(t *testing.T) {
	env := envWithSSO(t, "acme-sso", time.Time{}, false) // session exists, no token
	fe := asCoreError(t, NewResolver(env).explainCredentialFailure(sdkNoise))

	if fe.Code != CodeNoCredentials {
		t.Errorf("code = %q, want %q so the picker opens", fe.Code, CodeNoCredentials)
	}
	if !strings.Contains(fe.Remediation, "aws sso login") {
		t.Errorf("remediation = %q, want an sso login command", fe.Remediation)
	}
}

// Regression: the SDK's credential-chain message used to be passed straight
// through to the user.
func TestExplainNeverLeaksSDKInternals(t *testing.T) {
	forbidden := []string{"IMDS", "imds", "operation error", "refresh cached credentials",
		"AWS_EC2_METADATA_DISABLED", "get credentials"}

	for name, env := range map[string]TestEnv{
		"expired session": envWithSSO(t, "acme-sso", time.Now().Add(-time.Hour), true),
		"not signed in":   envWithSSO(t, "acme-sso", time.Time{}, false),
		"nothing at all":  {Vars: map[string]string{}, ConfigPath: "/nonexistent", CredsPath: "/nonexistent"},
	} {
		t.Run(name, func(t *testing.T) {
			fe := asCoreError(t, NewResolver(env).explainCredentialFailure(sdkNoise))
			for _, bad := range forbidden {
				if strings.Contains(fe.Message, bad) {
					t.Errorf("message leaks SDK internals (%q): %s", bad, fe.Message)
				}
			}
		})
	}
}

func TestExplainSDKErrorCodes(t *testing.T) {
	env := envWithSSO(t, "acme-sso", time.Now().Add(time.Hour), true) // valid token

	tests := []struct {
		code     string
		wantCode string
		wantExit int
	}{
		{"ExpiredToken", CodeAuthExpired, 3},
		{"ExpiredTokenException", CodeAuthExpired, 3},
		{"InvalidClientTokenId", CodeAuthExpired, 3},
		{"AccessDenied", CodePermission, 4},
		{"AccessDeniedException", CodePermission, 4},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			fe := asCoreError(t, NewResolver(env).explainCredentialFailure(&apiErr{tt.code}))
			if fe.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", fe.Code, tt.wantCode)
			}
			if fe.Exit != tt.wantExit {
				t.Errorf("exit = %d, want %d", fe.Exit, tt.wantExit)
			}
		})
	}
}

func TestExplainNothingConfigured(t *testing.T) {
	env := TestEnv{Vars: map[string]string{}, ConfigPath: "/nonexistent", CredsPath: "/nonexistent"}
	fe := asCoreError(t, NewResolver(env).explainCredentialFailure(sdkNoise))

	if fe.Code != CodeNoCredentials {
		t.Errorf("code = %q, want %q", fe.Code, CodeNoCredentials)
	}
}

// Regression: the bad-profile error used to say "run 'finops auth'" — advice
// that is circular when the user just ran finops auth.
func TestUnknownProfileNamesAvailableProfiles(t *testing.T) {
	fe := errUnknownProfile("nope", []string{"default", "prod"})

	if strings.Contains(fe.Remediation, "finops auth") {
		t.Errorf("remediation is circular: %q", fe.Remediation)
	}
	for _, want := range []string{"default", "prod"} {
		if !strings.Contains(fe.Remediation, want) {
			t.Errorf("remediation should list %q, got %q", want, fe.Remediation)
		}
	}
}

func TestUnknownProfileWithNoneConfigured(t *testing.T) {
	fe := errUnknownProfile("nope", nil)
	if !strings.Contains(fe.Remediation, "aws configure") {
		t.Errorf("with no profiles, remediation should point at aws configure, got %q", fe.Remediation)
	}
}

// The G7 invariant, enforced: an error without a remediation is a bug, because
// the remediation is what lets a human or an agent recover.
func TestEveryErrorCarriesRemediation(t *testing.T) {
	errs := map[string]*Error{
		"no credentials":  errNoCredentials(),
		"auth expired":    errAuthExpired("acme-sso", "42m"),
		"unknown profile": errUnknownProfile("x", []string{"default"}),
		"no profiles":     errUnknownProfile("x", nil),
		"aws error":       errAWS("doing a thing", sdkNoise),
	}
	for name, e := range errs {
		t.Run(name, func(t *testing.T) {
			if strings.TrimSpace(e.Remediation) == "" {
				t.Error("Remediation is empty")
			}
			if strings.TrimSpace(e.Message) == "" {
				t.Error("Message is empty")
			}
			if e.Code == "" {
				t.Error("Code is empty")
			}
			if e.Exit == 0 {
				t.Error("Exit is 0, which means success")
			}
		})
	}
}

// Regression: errAWS used to embed the raw SDK error in the user-facing
// message, so any fall-through path leaked the chain noise after all. A static
// -keys profile plus a generic failure is exactly such a path.
func TestFallthroughDoesNotLeakEither(t *testing.T) {
	dir := t.TempDir()
	creds := filepath.Join(dir, "credentials")
	if err := os.WriteFile(creds, []byte("[legacy]\naws_access_key_id = AKIA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := TestEnv{Vars: map[string]string{}, ConfigPath: "/nonexistent", CredsPath: creds}

	fe := asCoreError(t, NewResolver(env).explainCredentialFailure(sdkNoise))
	for _, bad := range []string{"IMDS", "operation error", "refresh cached credentials"} {
		if strings.Contains(fe.Message, bad) {
			t.Errorf("message leaks %q: %s", bad, fe.Message)
		}
	}
	if strings.TrimSpace(fe.Remediation) == "" {
		t.Error("remediation is empty")
	}
}
