package core

import (
	"os"
	"path/filepath"
)

// Env is the process environment and filesystem locations that credential
// discovery depends on. It exists as an interface so discovery can be tested
// against fixtures rather than whatever happens to be on the developer's
// machine — otherwise the tests pass or fail based on the tester's ~/.aws.
type Env interface {
	Get(key string) string
	SharedConfigFile() string
	SharedCredentialsFile() string
	SSOCacheDir() string
}

// OSEnv reads the real environment, honouring the AWS SDK's own overrides.
type OSEnv struct{}

func (OSEnv) Get(key string) string { return os.Getenv(key) }

func (e OSEnv) SharedConfigFile() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	return filepath.Join(e.home(), ".aws", "config")
}

func (e OSEnv) SharedCredentialsFile() string {
	if p := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); p != "" {
		return p
	}
	return filepath.Join(e.home(), ".aws", "credentials")
}

func (e OSEnv) SSOCacheDir() string {
	return filepath.Join(e.home(), ".aws", "sso", "cache")
}

func (OSEnv) home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

// TestEnv is an in-memory Env for tests.
type TestEnv struct {
	Vars        map[string]string
	ConfigPath  string
	CredsPath   string
	SSOCachePth string
}

func (e TestEnv) Get(key string) string         { return e.Vars[key] }
func (e TestEnv) SharedConfigFile() string      { return e.ConfigPath }
func (e TestEnv) SharedCredentialsFile() string { return e.CredsPath }
func (e TestEnv) SSOCacheDir() string           { return e.SSOCachePth }
