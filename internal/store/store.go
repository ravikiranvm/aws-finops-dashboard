// Package store persists audit results between runs.
//
// Without it the catalogue forgets everything on quit, so every launch claims
// nothing has ever been checked. It also underpins the "▼ $1,200 vs last run"
// comparison and the staleness marker (OVERHAUL.md D-A11).
//
// This is a cache, not configuration: written by the tool, freely refreshed,
// safe to delete. User intent belongs in a config file, and conflating the two
// is how a tool ends up silently editing someone's settings.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Run is one recorded scan of one check.
type Run struct {
	CheckID  string    `json:"check_id"`
	At       time.Time `json:"at"`
	Findings int       `json:"findings"`
	// USDPerMonth is the summed magnitude. Zero with findings > 0 means the
	// findings could not be priced — distinct from "found nothing".
	USDPerMonth float64  `json:"usd_per_month"`
	Priced      bool     `json:"priced"`
	Regions     []string `json:"regions,omitempty"`
}

// Age is how long ago the scan ran.
func (r Run) Age() time.Duration { return time.Since(r.At) }

// Stale reports whether a result is old enough that it should not be trusted
// without refreshing. Six hours is long enough to survive a working session and
// short enough that yesterday's numbers are visibly flagged.
func (r Run) Stale() bool { return r.Age() > 6*time.Hour }

// Cache is every recorded run for one account.
//
// Keyed by account so switching credentials cannot surface another account's
// findings — a correctness issue, not a tidiness one: seeing prod's waste while
// pointed at staging would be worse than seeing nothing.
type Cache struct {
	Account string         `json:"account"`
	Runs    map[string]Run `json:"runs"`
}

// Store reads and writes caches under a base directory.
type Store struct{ dir string }

// New returns a store rooted at the user's state directory, honouring
// XDG_STATE_HOME where set.
func New() *Store {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// No home directory is survivable: the tool works, it just cannot
			// remember. Point at a path that will fail to write and be ignored.
			return &Store{dir: filepath.Join(os.TempDir(), "finops")}
		}
		base = filepath.Join(home, ".local", "state")
	}
	return &Store{dir: filepath.Join(base, "finops")}
}

// NewAt returns a store rooted at an explicit directory, for tests.
func NewAt(dir string) *Store { return &Store{dir: dir} }

func (s *Store) path(account string) string {
	return filepath.Join(s.dir, "audit-"+account+".json")
}

// Load returns the cached runs for an account. A missing or unreadable cache
// is an empty cache, never an error: a corrupt file must not stop the tool from
// working, it should just mean nothing is remembered.
func (s *Store) Load(account string) Cache {
	c := Cache{Account: account, Runs: map[string]Run{}}

	raw, err := os.ReadFile(s.path(account))
	if err != nil {
		return c
	}
	var loaded Cache
	if json.Unmarshal(raw, &loaded) != nil || loaded.Runs == nil {
		return c
	}
	loaded.Account = account
	return loaded
}

// Save writes the cache, creating the directory if needed. Errors are returned
// but callers should treat them as non-fatal — failing to remember is a
// degradation, not a failure.
func (s *Store) Save(c Cache) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temp file and rename, so an interrupted write cannot leave a
	// half-written cache that fails to parse on next launch.
	tmp := s.path(c.Account) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write cache: %w", err)
	}
	return os.Rename(tmp, s.path(c.Account))
}

// Record adds or replaces one check's run and saves.
func (s *Store) Record(account string, run Run) error {
	c := s.Load(account)
	run.At = time.Now()
	c.Runs[run.CheckID] = run
	return s.Save(c)
}
