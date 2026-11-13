// Package core resolves who we are and which accounts we can reach.
package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Kind is a way credentials can arrive. These are the paths that actually
// matter in production: static keys in a shared credentials file are the
// individual/legacy case, not the norm (OVERHAUL.md D-B2).
type Kind string

const (
	KindEnv     Kind = "env"     // AWS_ACCESS_KEY_ID / AWS_SESSION_TOKEN
	KindSSO     Kind = "sso"     // IAM Identity Center
	KindProfile Kind = "profile" // named profile in the shared config
	KindAmbient Kind = "ambient" // IMDS, ECS task role, EKS IRSA
	KindOIDC    Kind = "oidc"    // web identity — GitHub Actions, GitLab CI
)

// Source is one candidate way to authenticate, as shown in the picker.
type Source struct {
	Kind Kind
	// Name identifies the specific source: a profile name or an SSO session
	// name. Empty for env and ambient, which are singular.
	Name string
	// Detail is the human summary shown beside the source ("4 found",
	// `session "acme-sso"`).
	Detail string
	// Available reports whether this source could be used right now.
	Available bool
	// Note explains a caveat or the reason it is unavailable — "expired 42m
	// ago", "not set", "not on EC2/ECS".
	Note string
	// Expiry is set for SSO sources with a cached token, zero otherwise.
	Expiry time.Time
}

// Expired reports whether a source has a known expiry that has passed.
func (s Source) Expired() bool {
	return !s.Expiry.IsZero() && time.Now().After(s.Expiry)
}

// Discovery is everything we could learn about available credentials without
// making a single AWS API call. Keeping it offline is what lets the picker
// render instantly.
type Discovery struct {
	Sources  []Source
	Profiles []string
	Sessions []string
}

// Discover inspects the environment and the shared AWS config to work out what
// credential sources exist. It never touches the network, so an unreachable
// IMDS endpoint cannot make the tool hang on startup.
func Discover(env Env) Discovery {
	d := Discovery{}

	cfg := parseSharedConfig(env.SharedConfigFile(), env.SharedCredentialsFile())
	d.Profiles = cfg.profiles
	d.Sessions = cfg.ssoSessions

	d.Sources = append(d.Sources, discoverEnv(env))
	d.Sources = append(d.Sources, discoverSSO(env, cfg)...)
	d.Sources = append(d.Sources, discoverProfiles(cfg))
	d.Sources = append(d.Sources, discoverAmbient(env))

	return d
}

// hasAmbient reports whether anything indicates we are running on an EC2
// instance, ECS task, or EKS pod. Used to skip the instance-metadata probe,
// which is slow and pointless anywhere else.
func (d Discovery) hasAmbient() bool {
	for _, s := range d.Sources {
		if s.Kind == KindAmbient && s.Available {
			return true
		}
	}
	return false
}

// Available returns only the usable sources, which is what the picker offers.
func (d Discovery) Available() []Source {
	var out []Source
	for _, s := range d.Sources {
		if s.Available {
			out = append(out, s)
		}
	}
	return out
}

func discoverEnv(env Env) Source {
	s := Source{Kind: KindEnv, Detail: "environment variables"}
	if env.Get("AWS_ACCESS_KEY_ID") != "" && env.Get("AWS_SECRET_ACCESS_KEY") != "" {
		s.Available = true
		s.Detail = "AWS_ACCESS_KEY_ID set"
		return s
	}
	// A web identity token is OIDC rather than static keys — different path,
	// same "it's in the environment" discovery.
	if env.Get("AWS_WEB_IDENTITY_TOKEN_FILE") != "" && env.Get("AWS_ROLE_ARN") != "" {
		return Source{Kind: KindOIDC, Detail: "web identity token", Available: true}
	}
	s.Note = "not set"
	return s
}

func discoverSSO(env Env, cfg sharedConfig) []Source {
	if len(cfg.ssoSessions) == 0 {
		return []Source{{
			Kind: KindSSO, Detail: "IAM Identity Center",
			Note: "no sso-session configured",
		}}
	}

	cached := readSSOTokenCache(env.SSOCacheDir())

	out := make([]Source, 0, len(cfg.ssoSessions))
	for _, name := range cfg.ssoSessions {
		s := Source{
			Kind:   KindSSO,
			Name:   name,
			Detail: fmt.Sprintf("session %q", name),
		}
		if exp, ok := cached[name]; ok {
			s.Expiry = exp
			if time.Now().After(exp) {
				// Still listed: the fix is one `aws sso login` away, so it is
				// the most useful thing to offer, not something to hide.
				s.Available = true
				s.Note = fmt.Sprintf("expired %s ago", humanDuration(time.Since(exp)))
			} else {
				s.Available = true
				s.Note = fmt.Sprintf("%s left", humanDuration(time.Until(exp)))
			}
		} else {
			s.Available = true
			s.Note = "not signed in"
		}
		out = append(out, s)
	}
	return out
}

func discoverProfiles(cfg sharedConfig) Source {
	s := Source{Kind: KindProfile, Detail: "shared profiles"}
	if n := len(cfg.profiles); n > 0 {
		s.Available = true
		s.Detail = fmt.Sprintf("%d found", n)
		return s
	}
	s.Note = "none configured"
	return s
}

func discoverAmbient(env Env) Source {
	s := Source{Kind: KindAmbient, Detail: "instance metadata"}
	switch {
	case env.Get("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "",
		env.Get("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "":
		s.Available = true
		s.Detail = "ECS task role"
	case env.Get("AWS_WEB_IDENTITY_TOKEN_FILE") != "":
		s.Available = true
		s.Detail = "EKS service account (IRSA)"
	default:
		// Deliberately not probing IMDS: the probe blocks for seconds when
		// off-instance, and startup latency is a stated requirement.
		s.Note = "not on EC2/ECS"
	}
	return s
}

// ---- shared config parsing -------------------------------------------------

type sharedConfig struct {
	profiles    []string
	ssoSessions []string
}

// parseSharedConfig extracts profile and sso-session names from the AWS shared
// config files. We only need section headers, so a full INI parser would be
// more dependency than the job warrants.
func parseSharedConfig(configPath, credentialsPath string) sharedConfig {
	seenProfile := map[string]bool{}
	seenSession := map[string]bool{}

	scanFile(configPath, func(section string) {
		switch {
		case section == "default":
			seenProfile["default"] = true
		case strings.HasPrefix(section, "profile "):
			seenProfile[strings.TrimSpace(strings.TrimPrefix(section, "profile "))] = true
		case strings.HasPrefix(section, "sso-session "):
			seenSession[strings.TrimSpace(strings.TrimPrefix(section, "sso-session "))] = true
		}
	})

	// The credentials file uses bare section names with no "profile " prefix.
	scanFile(credentialsPath, func(section string) {
		seenProfile[section] = true
	})

	return sharedConfig{profiles: sortedKeys(seenProfile), ssoSessions: sortedKeys(seenSession)}
}

func scanFile(path string, onSection func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return // absent or unreadable is normal, not an error
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if len(line) < 2 || line[0] != '[' || !strings.HasSuffix(line, "]") {
			continue
		}
		onSection(strings.TrimSpace(line[1 : len(line)-1]))
	}
}

// readSSOTokenCache reads token expiry from the SDK's own cache. We only read
// expiry — never the token itself — so the tool can say "expired 42m ago"
// without ever holding a credential (OVERHAUL.md D-B5, R-B10).
func readSSOTokenCache(dir string) map[string]time.Time {
	out := map[string]time.Time{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var tok struct {
			StartURL    string `json:"startUrl"`
			Region      string `json:"region"`
			ExpiresAt   string `json:"expiresAt"`
			SessionName string `json:"sessionName"`
		}
		if json.Unmarshal(raw, &tok) != nil || tok.ExpiresAt == "" {
			continue
		}
		exp, err := time.Parse(time.RFC3339, tok.ExpiresAt)
		if err != nil {
			continue
		}
		if tok.SessionName != "" {
			out[tok.SessionName] = exp
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// humanDuration renders a coarse, readable span: "42m", "6h 12m", "3d".
// Precision beyond this is noise in a status line.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
