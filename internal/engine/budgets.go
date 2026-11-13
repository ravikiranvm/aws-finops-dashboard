package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
)

// BudgetsAPI is the slice of the Budgets service this check needs.
type BudgetsAPI interface {
	DescribeBudgets(context.Context, *budgets.DescribeBudgetsInput, ...func(*budgets.Options)) (*budgets.DescribeBudgetsOutput, error)
}

// BudgetsBreached finds budgets whose actual spend has already passed their
// limit.
//
// It carries a count rather than a saving: a breached budget is a signal that
// something needs attention, not money we can offer to stop spending. Claiming
// a dollar figure here would be inventing one.
type BudgetsBreached struct{}

func (BudgetsBreached) CheckID() string { return "budget-breached" }

func (e BudgetsBreached) Evaluate(ctx context.Context, inv Inventory, _ Pricer) []audit.Finding {
	var findings []audit.Finding
	for _, b := range inv.Budgets {
		if f, ok := e.finding(inv.Account, b); ok {
			findings = append(findings, f)
		}
	}
	return findings
}

// finding builds one breach, or reports that this budget is not one.
func (e BudgetsBreached) finding(account string, b budgettypes.Budget) (audit.Finding, bool) {
	name := aws.ToString(b.BudgetName)

	limit, unit, ok := spend(b.BudgetLimit)
	if !ok {
		// RI and Savings Plans coverage budgets have no BudgetLimit — there is
		// nothing to be over. Not a finding, and not an error either.
		return audit.Finding{}, false
	}
	if b.CalculatedSpend == nil {
		// No spend calculated yet — a budget created moments ago. Nothing to
		// compare the limit against.
		return audit.Finding{}, false
	}
	actual, _, ok := spend(b.CalculatedSpend.ActualSpend)
	if !ok {
		return audit.Finding{}, false
	}
	if actual <= limit {
		return audit.Finding{}, false
	}

	over := actual - limit
	detail := map[string]any{
		"limit":       limit,
		"actual":      actual,
		"over":        over,
		"unit":        unit,
		"budget_type": string(b.BudgetType),
		"period":      period(b.TimeUnit),
	}

	// Forecast is genuinely optional: a budget in its first period has no
	// history to forecast from. Absent is different from zero, so the key is
	// only set when AWS actually returned one.
	evidence := []string{
		fmt.Sprintf("actual %s over a %s limit of %s",
			amount(actual, unit), period(b.TimeUnit), amount(limit, unit)),
		fmt.Sprintf("= %s over, %.0f%% of limit", amount(over, unit), actual/limit*100),
	}
	if forecast, _, ok := spend(b.CalculatedSpend.ForecastedSpend); ok {
		detail["forecast"] = forecast
		evidence = append(evidence,
			fmt.Sprintf("forecast to reach %s by period end", amount(forecast, unit)))
	}

	return audit.Finding{
		Fingerprint: audit.NewFingerprint(e.CheckID(), account, "global", name),
		CheckID:     e.CheckID(),
		Service:     "Budgets",
		Category:    audit.CategoryRisk,
		Account:     account,
		// Budgets are account-wide behind one global endpoint, so there is no
		// region to attribute this to. Saying "us-east-1" because that is where
		// the endpoint lives would be misleading.
		Region:     "global",
		ResourceID: name,
		Title: fmt.Sprintf("%s · %s of %s spent", name,
			amount(actual, unit), amount(limit, unit)),
		Magnitude: audit.Magnitude{
			Kind:  audit.MagnitudeCount,
			Basis: audit.BasisNone,
			Count: 1,
		},
		Detail:   detail,
		Evidence: evidence,
	}, true
}

// spend parses an SDK Spend into a float and its unit.
//
// The SDK models amounts as *string, not a number, so every amount has to be
// parsed. A parse failure returns false rather than zero: treating an
// unreadable amount as $0.00 would turn a breached budget into a healthy one.
func spend(s *budgettypes.Spend) (float64, string, bool) {
	if s == nil || s.Amount == nil {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(*s.Amount, 64)
	if err != nil {
		return 0, "", false
	}
	unit := aws.ToString(s.Unit)
	if unit == "" {
		unit = "USD"
	}
	return v, unit, true
}

// amount renders a value in its own unit. Only USD gets a dollar sign: a budget
// denominated in GB or in another currency must not be printed as dollars.
//
// Cents are always kept here, unlike the table, which rounds above $100. This
// is the working shown to justify the number, so it should reconcile exactly
// with what AWS reports.
func amount(v float64, unit string) string {
	if unit == "USD" {
		return "$" + grouped(v)
	}
	return grouped(v) + " " + unit
}

// grouped formats a number with thousands separators. Without them a four- or
// five-figure budget is a wall of digits, and comparing 13204.55 to 12000.00 at
// a glance means counting characters.
func grouped(v float64) string {
	whole := fmt.Sprintf("%.2f", v)
	dot := strings.IndexByte(whole, '.')
	intPart, frac := whole[:dot], whole[dot:]

	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")

	var out []byte
	for i := 0; i < len(intPart); i++ {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, intPart[i])
	}
	if neg {
		return "-" + string(out) + frac
	}
	return string(out) + frac
}

// period shortens the SDK's time unit for the table.
func period(t budgettypes.TimeUnit) string {
	switch t {
	case budgettypes.TimeUnitDaily:
		return "daily"
	case budgettypes.TimeUnitMonthly:
		return "monthly"
	case budgettypes.TimeUnitQuarterly:
		return "quarterly"
	case budgettypes.TimeUnitAnnually:
		return "annual"
	}
	return string(t)
}

// allBudgets pages through every budget in the account.
//
// v2 issued a single unpaginated call, so an account with more than 100 budgets
// silently reported only the first hundred. It also swallowed every error,
// which made "no permission" and "no budgets" look identical.
func allBudgets(ctx context.Context, api BudgetsAPI, accountID string) ([]budgettypes.Budget, error) {
	var out []budgettypes.Budget
	var token *string

	for {
		page, err := api.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
			AccountId: aws.String(accountID),
			NextToken: token,
		})
		if err != nil {
			// Partial pages are kept: a truncated set the caller knows about
			// beats an empty set it does not.
			return out, fmt.Errorf("describe budgets: %w", err)
		}
		out = append(out, page.Budgets...)

		token = page.NextToken
		if token == nil {
			return out, nil
		}
	}
}
