package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
)

// budget builds a fixture. Amounts are strings because that is how the SDK
// models them — see spend().
func budget(name, limit, actual string, opts ...func(*budgettypes.Budget)) budgettypes.Budget {
	b := budgettypes.Budget{
		BudgetName: aws.String(name),
		BudgetType: budgettypes.BudgetTypeCost,
		TimeUnit:   budgettypes.TimeUnitMonthly,
		BudgetLimit: &budgettypes.Spend{
			Amount: aws.String(limit), Unit: aws.String("USD"),
		},
		CalculatedSpend: &budgettypes.CalculatedSpend{
			ActualSpend: &budgettypes.Spend{
				Amount: aws.String(actual), Unit: aws.String("USD"),
			},
		},
	}
	for _, o := range opts {
		o(&b)
	}
	return b
}

func evalBudgets(t *testing.T, bs ...budgettypes.Budget) []string {
	t.Helper()
	fs := BudgetsBreached{}.Evaluate(context.Background(),
		Inventory{Account: "548846591939", Region: "global", Budgets: bs}, nil)

	names := make([]string, 0, len(fs))
	for _, f := range fs {
		names = append(names, f.ResourceID)
	}
	return names
}

func TestBudgetsOnlyReportsBreaches(t *testing.T) {
	got := evalBudgets(t,
		budget("under", "1000", "400"),
		budget("exactly-at-limit", "1000", "1000"),
		budget("over", "1000", "1204.55"),
	)

	// At the limit is not over it. A budget that lands exactly on its number is
	// working as intended, and flagging it would train people to ignore this.
	if len(got) != 1 || got[0] != "over" {
		t.Errorf("breaches = %v, want [over]", got)
	}
}

func TestBudgetUnparseableAmountIsNotTreatedAsZero(t *testing.T) {
	// v2 coerced amounts with float(); anything unparseable would have become
	// 0.0, turning a breached budget into a healthy one. Skipping is the honest
	// failure: we could not read it, so we do not claim it is fine or broken.
	got := evalBudgets(t, budget("garbage", "1000", "not-a-number"))
	if len(got) != 0 {
		t.Errorf("unreadable amount produced %v, want no finding", got)
	}
}

func TestBudgetWithoutLimitIsSkipped(t *testing.T) {
	// RI and Savings Plans coverage budgets carry no BudgetLimit. There is
	// nothing to be over, and dereferencing it is how v2 would have panicked.
	noLimit := budget("ri-coverage", "1", "5", func(b *budgettypes.Budget) {
		b.BudgetLimit = nil
		b.BudgetType = budgettypes.BudgetTypeRICoverage
	})
	noSpend := budget("brand-new", "1000", "1", func(b *budgettypes.Budget) {
		b.CalculatedSpend = nil
	})

	if got := evalBudgets(t, noLimit, noSpend); len(got) != 0 {
		t.Errorf("got %v, want no findings", got)
	}
}

func TestBudgetNonUSDIsNotPrintedAsDollars(t *testing.T) {
	// A usage budget is denominated in GB, not dollars. Rendering "$512.00"
	// for 512 GB would be a fabricated cost figure.
	gb := budget("s3-usage", "500", "512", func(b *budgettypes.Budget) {
		b.BudgetType = budgettypes.BudgetTypeUsage
		b.BudgetLimit.Unit = aws.String("GB")
		b.CalculatedSpend.ActualSpend.Unit = aws.String("GB")
	})

	fs := BudgetsBreached{}.Evaluate(context.Background(),
		Inventory{Account: "1", Budgets: []budgettypes.Budget{gb}}, nil)
	if len(fs) != 1 {
		t.Fatalf("got %d findings, want 1", len(fs))
	}
	if got := fs[0].Detail["unit"]; got != "GB" {
		t.Errorf("unit = %v, want GB", got)
	}
	for _, e := range fs[0].Evidence {
		if len(e) > 0 && e[0] == '$' {
			t.Errorf("evidence renders a GB budget as dollars: %q", e)
		}
	}
}

func TestBudgetCarriesCountNotSavings(t *testing.T) {
	fs := BudgetsBreached{}.Evaluate(context.Background(),
		Inventory{Account: "1", Budgets: []budgettypes.Budget{budget("over", "100", "150")}}, nil)

	// A breached budget is a signal, not money we can offer to stop spending.
	// Priced() must stay false so it can never be added into a savings total.
	if fs[0].Magnitude.Priced() {
		t.Error("a budget breach must not report a dollar saving")
	}
	if fs[0].Magnitude.Count != 1 {
		t.Errorf("count = %d, want 1", fs[0].Magnitude.Count)
	}
	if fs[0].Region != "global" {
		t.Errorf("region = %q, want global", fs[0].Region)
	}
}

func TestBudgetForecastAbsentIsNotZero(t *testing.T) {
	withF := budget("has-forecast", "100", "150", func(b *budgettypes.Budget) {
		b.CalculatedSpend.ForecastedSpend = &budgettypes.Spend{
			Amount: aws.String("300"), Unit: aws.String("USD"),
		}
	})
	fs := BudgetsBreached{}.Evaluate(context.Background(),
		Inventory{Account: "1", Budgets: []budgettypes.Budget{withF, budget("none", "100", "150")}}, nil)

	if _, ok := fs[0].Detail["forecast"]; !ok {
		t.Error("a budget with a forecast should carry it")
	}
	// A budget in its first period has nothing to forecast from. Recording 0
	// would read as "forecast to spend nothing", which is the opposite.
	if _, ok := fs[1].Detail["forecast"]; ok {
		t.Error("a budget with no forecast must not report one")
	}
}

// fakeBudgets pages, and can fail on a chosen page.
type fakeBudgets struct {
	pages   [][]budgettypes.Budget
	failOn  int
	calls   int
	lastAcc string
}

func (f *fakeBudgets) DescribeBudgets(_ context.Context, in *budgets.DescribeBudgetsInput, _ ...func(*budgets.Options)) (*budgets.DescribeBudgetsOutput, error) {
	f.lastAcc = aws.ToString(in.AccountId)
	i := f.calls
	f.calls++
	if f.failOn == i {
		return nil, errors.New("AccessDeniedException")
	}
	out := &budgets.DescribeBudgetsOutput{Budgets: f.pages[i]}
	if i < len(f.pages)-1 {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

func TestAllBudgetsPaginates(t *testing.T) {
	// v2 made a single call, so accounts with more than 100 budgets silently
	// reported the first hundred as if that were all of them.
	f := &fakeBudgets{failOn: -1, pages: [][]budgettypes.Budget{
		{budget("a", "1", "2"), budget("b", "1", "2")},
		{budget("c", "1", "2")},
	}}

	got, err := allBudgets(context.Background(), f, "548846591939")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d budgets across %d calls, want 3", len(got), f.calls)
	}
	if f.lastAcc != "548846591939" {
		t.Errorf("account = %q, want the caller's", f.lastAcc)
	}
}

func TestAllBudgetsKeepsPartialPagesOnError(t *testing.T) {
	f := &fakeBudgets{failOn: 1, pages: [][]budgettypes.Budget{
		{budget("a", "1", "2")}, nil,
	}}

	got, err := allBudgets(context.Background(), f, "1")
	// v2 swallowed this entirely, so a denied permission and an account with no
	// budgets produced the same empty result.
	if err == nil {
		t.Fatal("a failed page must surface as an error")
	}
	if len(got) != 1 {
		t.Errorf("got %d budgets, want the 1 fetched before the failure", len(got))
	}
}
