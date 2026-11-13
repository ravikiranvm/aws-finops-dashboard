// Package audit defines what the tool can look for and how findings are described.
//
// Checks are declared as data, not code (OVERHAUL.md D-X9). The catalogue UI,
// the settings screen, the scan planner, the docs, and `finops schema` output
// are all derived from these declarations. Adding a check means adding one
// entry plus an evaluator — never editing a parallel list somewhere else.
package audit

import "fmt"

// Category is the kind of question a finding answers. Loosely aligned to the
// FinOps Framework, which is used as guidance rather than gospel.
type Category string

const (
	// CategoryWaste: money being spent for nothing. Stop spending it.
	CategoryWaste Category = "waste"
	// CategoryRate: paying more than necessary for the same usage.
	CategoryRate Category = "rate"
	// CategoryAllocation: spend that cannot be attributed to an owner.
	CategoryAllocation Category = "allocation"
	// CategoryPolicy: our own rules are not being followed.
	CategoryPolicy Category = "policy"
	// CategoryRisk: something financial is about to go wrong.
	CategoryRisk Category = "risk"
)

// Order is the display order of categories, highest impact first.
var Order = []Category{
	CategoryWaste, CategoryRate, CategoryAllocation, CategoryPolicy, CategoryRisk,
}

func (c Category) Title() string {
	switch c {
	case CategoryWaste:
		return "Waste"
	case CategoryRate:
		return "Rate"
	case CategoryAllocation:
		return "Allocation"
	case CategoryPolicy:
		return "Policy"
	case CategoryRisk:
		return "Financial risk"
	}
	return string(c)
}

// Basis describes how much to trust a savings figure. It is always displayed
// alongside the number: a wrong dollar amount in a budget meeting is worse
// than no dollar amount.
type Basis string

const (
	// BasisExact is arithmetic from size, type, and published price.
	BasisExact Basis = "exact"
	// BasisConditional is exact arithmetic under an assumption the user must
	// accept, e.g. "if you would stop this instance".
	BasisConditional Basis = "conditional"
	// BasisApproximate has components we cannot price precisely.
	BasisApproximate Basis = "approximate"
	// BasisNone means the finding carries no savings figure at all.
	BasisNone Basis = "none"
)

// MagnitudeKind keeps incommensurable numbers from being summed together.
// Savings and unattributed spend are both dollars, but adding them is
// meaningless.
type MagnitudeKind string

const (
	// MagnitudeSavings is money that would stop being spent. Waste and Rate.
	MagnitudeSavings MagnitudeKind = "savings_monthly_usd"
	// MagnitudeUnallocated is spend that cannot be attributed. Allocation.
	MagnitudeUnallocated MagnitudeKind = "unallocated_monthly_usd"
	// MagnitudeCount is a bare count. Policy and Risk.
	MagnitudeCount MagnitudeKind = "count"
)

// Scope is how widely a check has to be run to see everything.
//
// It belongs on the declaration because the scan planner derives the fan-out
// from it: getting this wrong does not fail, it silently reports the same
// account-wide resource once per region.
type Scope string

const (
	// ScopeRegional means the resources live in a region and every enabled
	// region must be visited. The default, and true of everything in EC2.
	ScopeRegional Scope = "regional"
	// ScopeAccount means one call per account covers it. Budgets, IAM and
	// Organizations are account-wide with a single global endpoint.
	ScopeAccount Scope = "account"
)

// Format is how the render layer should present a column's value. Declared
// rather than inferred from the Go type, because a float64 could be dollars,
// gigabytes, or a percentage and only the check knows which.
type Format string

const (
	// FormatText prints the value as-is.
	FormatText Format = ""
	// FormatMoney is a USD amount.
	FormatMoney Format = "money"
	// FormatAmount is an amount in the finding's own unit, read from
	// Detail["unit"] — a budget may be denominated in something other than USD,
	// and printing "$" in front of it would be a lie.
	FormatAmount Format = "amount"
	// FormatGB is a size in gigabytes.
	FormatGB Format = "gb"
)

// Column declares one column of a check's findings table.
//
// Findings differ too much for one table: a volume has a size and a type, a
// budget has a limit and an overspend. Declaring the columns with the check
// keeps that knowledge in one place instead of a type switch in the renderer.
type Column struct {
	// Key indexes Finding.Detail. Empty means the resource ID itself.
	Key    string
	Header string
	// Width in columns; ignored when Flex.
	Width int
	// Flex absorbs whatever width is left over. Exactly one column per check
	// should set it, and it is the one that gets truncated when space runs out.
	Flex bool
	// Right-aligns the column. Numbers read better right-aligned; text does not.
	Right bool
	// Shed orders which columns are dropped on a narrow terminal: higher goes
	// first, zero never drops. The column carrying the point of the check must
	// stay at zero.
	Shed int

	Format Format
}

// Check declares one thing the tool knows how to look for.
type Check struct {
	// ID is stable and machine-facing; it appears in config, suppressions,
	// and JSON output. Never change one after release.
	ID string
	// Name is the human label shown in the catalogue.
	Name string
	// Description is one or two sentences shown in the catalogue's detail
	// strip: what it looks for and why it costs money.
	Description string
	// Service is the AWS service grouping, e.g. "EBS", "EC2".
	Service string

	Category  Category
	Basis     Basis
	Magnitude MagnitudeKind

	// APIs lists the AWS API calls this check needs. The scan planner uses
	// this to deduplicate work across checks: several checks share
	// ec2:DescribeVolumes, and it must be fetched once, not once each.
	APIs []string

	// DefaultEnabled controls whether the check runs without configuration.
	DefaultEnabled bool

	// CountLabel is the noun a count-carrying check reports, e.g. "breached".
	// The catalogue shows "1 breached" rather than a bare "1", which beside a
	// column of dollar figures reads as an amount. Required for count checks.
	CountLabel string

	// Scope is how far the check has to be run to see everything. Empty means
	// ScopeRegional.
	Scope Scope

	// Columns declare the findings table. Empty until the check has an
	// evaluator — there is nothing to tabulate before then.
	Columns []Column
}

// Fanout reports whether the check needs running in every region, or once per
// account.
func (c Check) Fanout() Scope {
	if c.Scope == "" {
		return ScopeRegional
	}
	return c.Scope
}

// Validate reports whether a declaration is internally consistent. Called by
// the registry at init so a malformed check fails loudly at startup rather
// than producing a nonsensical screen later.
func (c Check) Validate() error {
	if c.ID == "" || c.Name == "" || c.Service == "" {
		return fmt.Errorf("check %q: ID, Name and Service are required", c.ID)
	}
	if len(c.APIs) == 0 {
		return fmt.Errorf("check %q: at least one API must be declared", c.ID)
	}

	switch c.Scope {
	case "", ScopeRegional, ScopeAccount:
	default:
		return fmt.Errorf("check %q: unknown scope %q", c.ID, c.Scope)
	}

	if len(c.Columns) > 0 {
		flex := 0
		for _, col := range c.Columns {
			if col.Flex {
				flex++
			}
		}
		// Without a flexible column the table cannot fill the frame; with two
		// the leftover width has no single owner and the arithmetic silently
		// splits it.
		if flex != 1 {
			return fmt.Errorf("check %q: needs exactly one flexible column, has %d", c.ID, flex)
		}
	}

	switch c.Category {
	case CategoryWaste, CategoryRate:
		if c.Magnitude != MagnitudeSavings {
			return fmt.Errorf("check %q: %s findings must carry savings", c.ID, c.Category)
		}
		if c.Basis == BasisNone {
			return fmt.Errorf("check %q: %s findings need a basis for their savings", c.ID, c.Category)
		}
	case CategoryAllocation:
		if c.Magnitude != MagnitudeUnallocated {
			return fmt.Errorf("check %q: allocation findings carry unattributed spend, not savings", c.ID)
		}
	case CategoryPolicy, CategoryRisk:
		if c.Magnitude != MagnitudeCount {
			return fmt.Errorf("check %q: %s findings carry a count", c.ID, c.Category)
		}
		if c.Basis != BasisNone {
			return fmt.Errorf("check %q: %s findings have no savings, so basis must be none", c.ID, c.Category)
		}
		if c.CountLabel == "" {
			return fmt.Errorf("check %q: count findings need a CountLabel to say what is being counted", c.ID)
		}
	default:
		return fmt.Errorf("check %q: unknown category %q", c.ID, c.Category)
	}
	return nil
}
