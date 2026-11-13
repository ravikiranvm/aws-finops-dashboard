package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Finding is one thing worth knowing about, found by one check.
//
// The shape matters: Service, Category, and Account are orthogonal dimensions,
// which is what lets the same records be pivoted by service, by category, or by
// owner without re-querying anything (OVERHAUL.md §C.0). v2 baked its
// presentation into its data — "untagged resources" was a table column, so it
// could only ever be a table column.
type Finding struct {
	// Fingerprint identifies this finding across runs, so a suppression made
	// today still applies tomorrow. Derived, not stored.
	Fingerprint string

	CheckID  string
	Service  string
	Category Category

	Account    string // 12-digit ID
	Region     string // "global" for account-wide resources
	ResourceID string

	// Title is the one-line human summary.
	Title string

	Magnitude Magnitude

	// Owner is derived from tags where possible. A finding nobody owns cannot
	// be actioned, so the absence is itself information.
	Owner string

	// Detail carries check-specific fields for the resource view — size, age,
	// volume type, and so on. Deliberately loose: every check needs different
	// fields, and forcing them into a common struct would flatten them all to
	// the lowest common denominator.
	Detail map[string]any

	// Evidence is the human-readable arithmetic behind Magnitude, shown in the
	// resource view. Showing the working is the honesty mechanism: anyone can
	// check the number rather than trusting it.
	Evidence []string

	FirstSeen time.Time
}

// Magnitude is how much a finding matters, typed so incommensurable numbers
// cannot be summed. Savings and unattributed spend are both dollars; adding
// them together is meaningless.
type Magnitude struct {
	Kind  MagnitudeKind
	Basis Basis

	// USDPerMonth is set for savings and unallocated kinds.
	USDPerMonth float64
	// Count is set for policy and risk kinds.
	Count int
}

// Priced reports whether a dollar figure is actually available. A check can run
// successfully without pricing — if pricing:GetProducts is denied we still
// report the 8 unattached volumes, just without the money, flagged as such.
// That beats failing, and it beats showing $0.
func (m Magnitude) Priced() bool {
	return (m.Kind == MagnitudeSavings || m.Kind == MagnitudeUnallocated) && m.USDPerMonth > 0
}

// NewFingerprint builds a stable identifier from the parts that make a finding
// the same finding: which check, which account, which region, which resource.
// Deliberately excludes size and price, so a volume growing from 100GB to 200GB
// stays the same finding rather than appearing as a new one.
func NewFingerprint(checkID, account, region, resourceID string) string {
	sum := sha256.Sum256([]byte(checkID + "\x00" + account + "\x00" + region + "\x00" + resourceID))
	return hex.EncodeToString(sum[:8])
}

// Coverage records what was actually scanned versus what was asked for.
//
// Without this a total is a lie: "$2,847 of waste" reads as complete even when
// two regions were denied. It is threaded through the engine rather than bolted
// on afterwards, for the same reason the cost metric is a required field.
type Coverage struct {
	AccountsRequested int
	AccountsScanned   int
	RegionsRequested  int
	RegionsScanned    int
	ChecksRequested   int
	ChecksRun         int
	// ChecksSkipped maps check ID to why it did not run — a denied permission,
	// usually. Skipping silently would make an empty result indistinguishable
	// from a clean one.
	ChecksSkipped map[string]string
}

// Complete reports whether everything requested was actually covered.
func (c Coverage) Complete() bool {
	return c.AccountsScanned == c.AccountsRequested &&
		c.RegionsScanned == c.RegionsRequested &&
		c.ChecksRun == c.ChecksRequested
}

// Summary describes coverage in one line, naming only the dimensions that are
// actually short. Reporting "1 of 1 regions · partial" is worse than saying
// nothing: it looks like a contradiction and hides which dimension fell short.
func (c Coverage) Summary() string {
	var short []string
	if c.AccountsScanned != c.AccountsRequested {
		short = append(short, fmt.Sprintf("%d of %d accounts", c.AccountsScanned, c.AccountsRequested))
	}
	if c.RegionsScanned != c.RegionsRequested {
		short = append(short, fmt.Sprintf("%d of %d regions", c.RegionsScanned, c.RegionsRequested))
	}
	if c.ChecksRun != c.ChecksRequested {
		short = append(short, fmt.Sprintf("%d of %d checks", c.ChecksRun, c.ChecksRequested))
	}

	if len(short) == 0 {
		return fmt.Sprintf("%d account(s) · %d region(s) · %d checks",
			c.AccountsScanned, c.RegionsScanned, c.ChecksRun)
	}
	return strings.Join(short, " · ") + " · partial"
}

// SkippedIDs returns skipped check IDs in a stable order. Map iteration order
// is random in Go, so unsorted output would differ between identical runs and
// break golden-frame comparisons.
func (c Coverage) SkippedIDs() []string {
	out := make([]string, 0, len(c.ChecksSkipped))
	for id := range c.ChecksSkipped {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Failure is one thing that went wrong without stopping the run. A single
// inaccessible account degrades in place and is excluded from totals; it never
// aborts the scan (OVERHAUL.md R-B14).
type Failure struct {
	Account string
	Region  string
	CheckID string
	Message string
}
