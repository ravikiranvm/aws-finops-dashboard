package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeAWSConfig lays down a shared config file and returns an Env pointing at
// it, so tests never read the developer's real ~/.aws.
func writeAWSConfig(t *testing.T, config, credentials string, vars map[string]string) TestEnv {
	t.Helper()
	dir := t.TempDir()

	cfgPath := filepath.Join(dir, "config")
	if config != "" {
		if err := os.WriteFile(cfgPath, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	credPath := filepath.Join(dir, "credentials")
	if credentials != "" {
		if err := os.WriteFile(credPath, []byte(credentials), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if vars == nil {
		vars = map[string]string{}
	}
	return TestEnv{
		Vars:        vars,
		ConfigPath:  cfgPath,
		CredsPath:   credPath,
		SSOCachePth: filepath.Join(dir, "sso-cache"),
	}
}

func find(d Discovery, kind Kind) (Source, bool) {
	for _, s := range d.Sources {
		if s.Kind == kind {
			return s, true
		}
	}
	return Source{}, false
}

func TestDiscoverProfiles(t *testing.T) {
	env := writeAWSConfig(t, `
[default]
region = us-east-1

[profile prod]
region = us-east-1

[profile staging]
region = eu-west-1

[sso-session acme-sso]
sso_start_url = https://acme.awsapps.com/start
`, "", nil)

	d := Discover(env)

	want := []string{"default", "prod", "staging"}
	if len(d.Profiles) != len(want) {
		t.Fatalf("profiles = %v, want %v", d.Profiles, want)
	}
	for i := range want {
		if d.Profiles[i] != want[i] {
			t.Errorf("profiles[%d] = %q, want %q", i, d.Profiles[i], want[i])
		}
	}

	if len(d.Sessions) != 1 || d.Sessions[0] != "acme-sso" {
		t.Errorf("sessions = %v, want [acme-sso]", d.Sessions)
	}

	p, _ := find(d, KindProfile)
	if !p.Available || p.Detail != "3 found" {
		t.Errorf("profile source = %+v, want available with '3 found'", p)
	}
}

func TestDiscoverCredentialsFileUsesBareSections(t *testing.T) {
	// The credentials file has no "profile " prefix — a real difference from
	// the config file that is easy to get wrong.
	env := writeAWSConfig(t, "", "[legacy]\naws_access_key_id = AKIA...\n", nil)

	d := Discover(env)
	if len(d.Profiles) != 1 || d.Profiles[0] != "legacy" {
		t.Errorf("profiles = %v, want [legacy]", d.Profiles)
	}
}

func TestDiscoverEnvStaticKeys(t *testing.T) {
	env := writeAWSConfig(t, "", "", map[string]string{
		"AWS_ACCESS_KEY_ID":     "AKIAEXAMPLE",
		"AWS_SECRET_ACCESS_KEY": "secret",
	})

	s, ok := find(Discover(env), KindEnv)
	if !ok || !s.Available {
		t.Errorf("env source = %+v, want available", s)
	}
}

func TestDiscoverEnvHalfSetIsNotAvailable(t *testing.T) {
	// A key without a secret is a misconfiguration, not a usable source.
	env := writeAWSConfig(t, "", "", map[string]string{"AWS_ACCESS_KEY_ID": "AKIAEXAMPLE"})

	s, _ := find(Discover(env), KindEnv)
	if s.Available {
		t.Error("env source should not be available with only an access key id")
	}
}

func TestDiscoverOIDC(t *testing.T) {
	env := writeAWSConfig(t, "", "", map[string]string{
		"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/secrets/token",
		"AWS_ROLE_ARN":                "arn:aws:iam::123456789012:role/ci",
	})

	s, ok := find(Discover(env), KindOIDC)
	if !ok || !s.Available {
		t.Errorf("oidc source = %+v, want available", s)
	}
}

func TestDiscoverAmbient(t *testing.T) {
	tests := []struct {
		name      string
		vars      map[string]string
		available bool
		detail    string
	}{
		{"ecs task role", map[string]string{
			"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/v2/credentials/abc",
		}, true, "ECS task role"},
		{"eks irsa", map[string]string{
			"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/run/secrets/token",
		}, true, "EKS service account (IRSA)"},
		{"laptop", nil, false, "instance metadata"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := find(Discover(writeAWSConfig(t, "", "", tt.vars)), KindAmbient)
			if s.Available != tt.available {
				t.Errorf("available = %v, want %v", s.Available, tt.available)
			}
			if s.Detail != tt.detail {
				t.Errorf("detail = %q, want %q", s.Detail, tt.detail)
			}
		})
	}
}

func TestDiscoverSSOExpiry(t *testing.T) {
	env := writeAWSConfig(t, "[sso-session acme-sso]\nsso_start_url = https://x\n", "", nil)

	if err := os.MkdirAll(env.SSOCachePth, 0o700); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-42 * time.Minute).UTC().Format(time.RFC3339)
	token := fmt.Sprintf(`{"sessionName":"acme-sso","expiresAt":%q,"region":"us-east-1"}`, expired)
	if err := os.WriteFile(filepath.Join(env.SSOCachePth, "tok.json"), []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}

	s, ok := find(Discover(env), KindSSO)
	if !ok {
		t.Fatal("no sso source discovered")
	}
	// An expired session stays available: the fix is one command away, so
	// offering it is more useful than hiding it.
	if !s.Available {
		t.Error("expired sso session should still be offered")
	}
	if !s.Expired() {
		t.Error("Expired() should be true")
	}
	if s.Note != "expired 42m ago" {
		t.Errorf("note = %q, want %q", s.Note, "expired 42m ago")
	}
}

func TestDiscoverSSOValidToken(t *testing.T) {
	env := writeAWSConfig(t, "[sso-session acme-sso]\nsso_start_url = https://x\n", "", nil)
	if err := os.MkdirAll(env.SSOCachePth, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := time.Now().Add(6*time.Hour + 12*time.Minute).UTC().Format(time.RFC3339)
	token := fmt.Sprintf(`{"sessionName":"acme-sso","expiresAt":%q}`, valid)
	os.WriteFile(filepath.Join(env.SSOCachePth, "tok.json"), []byte(token), 0o600)

	s, _ := find(Discover(env), KindSSO)
	if s.Expired() {
		t.Error("valid token should not report expired")
	}
	if s.Note != "6h 11m left" && s.Note != "6h 12m left" { // tolerate the tick
		t.Errorf("note = %q, want ~'6h 12m left'", s.Note)
	}
}

func TestDiscoverNothingConfigured(t *testing.T) {
	d := Discover(writeAWSConfig(t, "", "", nil))
	if got := len(d.Available()); got != 0 {
		t.Errorf("Available() = %d sources, want 0: %+v", got, d.Available())
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "under a minute"},
		{42 * time.Minute, "42m"},
		{2 * time.Hour, "2h"},
		{6*time.Hour + 12*time.Minute, "6h 12m"},
		{50 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		if got := humanDuration(tt.d); got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
