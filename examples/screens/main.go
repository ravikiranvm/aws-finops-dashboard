// Command screens renders each TUI screen once and prints it, for design
// review and for eyeballing a change without launching the real tool.
//
// It calls View() directly rather than running a Bubble Tea program, which
// matters: the v2 renderer emits incremental, cursor-positioned updates, so
// capturing a running program and stripping ANSI produces convincing garbage.
// Printing the view is the honest way to see a frame.
//
//	go run ./examples/screens            # all screens
//	go run ./examples/screens picker     # one screen
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	tuiaudit "github.com/ravikiranvm/aws-finops-dashboard/internal/tui/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/auth"
)

func main() {
	want := ""
	if len(os.Args) > 1 {
		want = os.Args[1]
	}
	if len(os.Args) > 3 {
		fmt.Sscanf(os.Args[2], "%d", &termW)
		fmt.Sscanf(os.Args[3], "%d", &termH)
	}

	for _, s := range screens() {
		if want != "" && want != s.name {
			continue
		}
		fmt.Printf("\n\x1b[2m── %s %s\x1b[0m\n\n", s.name, strings.Repeat("─", max(0, 60-len(s.name))))
		fmt.Println(s.render())
	}
}

type screen struct {
	name   string
	render func() string
}

func screens() []screen {
	return []screen{
		{"picker", func() string {
			return view(auth.NewPicker(laptopWithExpiredSSO(), "3.0"))
		}},
		{"picker-empty", func() string {
			return view(auth.NewPicker(nothingConfigured(), "3.0"))
		}},
		{"audit-catalog", func() string {
			return view(tuiaudit.NewCatalog(nil, "3.0", "raviki · 548846591939", nil))
		}},
		{"audit-catalog-run", func() string {
			return view(tuiaudit.NewCatalog(afterARun(), "3.0", "prod-mgmt · org 14", nil))
		}},
		{"audit-results", func() string {
			// Two 'j's land on Unattached EBS volumes, which declares a Fix, so
			// the strip shows the remediation command.
			return viewAfter(tuiaudit.NewCatalog(nil, "3.0", "raviki · 548846591939", stubRunner(1)),
				tea.KeyPressMsg{Code: 'j'}, tea.KeyPressMsg{Code: 'j'}, tea.KeyPressMsg{Code: 'r'})
		}},
		{"audit-budgets", func() string {
			// Ten moves reach Budgets over their limit, the only risk-category
			// check, which has its own columns and no savings figure.
			keys := []tea.Msg{}
			for i := 0; i < 10; i++ {
				keys = append(keys, tea.KeyPressMsg{Code: 'j'})
			}
			keys = append(keys, tea.KeyPressMsg{Code: 'r'})
			return viewAfter(tuiaudit.NewCatalog(nil, "3.0", "raviki · 548846591939", budgetRunner()), keys...)
		}},
		{"audit-results-scrolled", func() string {
			keys := []tea.Msg{tea.KeyPressMsg{Code: 'j'}, tea.KeyPressMsg{Code: 'j'}, tea.KeyPressMsg{Code: 'r'}}
			// Walk down far enough that the list has scrolled off its first row.
			for i := 0; i < 11; i++ {
				keys = append(keys, tea.KeyPressMsg{Code: 'j'})
			}
			return viewAfter(tuiaudit.NewCatalog(nil, "3.0", "raviki · 548846591939", stubRunner(40)), keys...)
		}},
	}
}

// stubRunner returns n fixture findings so the results screen can be rendered
// without AWS. The first one's numbers match a real run against a small
// account; the rest exist to make the list overflow.
func stubRunner(n int) tuiaudit.RunFunc {
	regions := []string{"ap-southeast-1", "us-east-1", "eu-west-1", "ap-south-1"}
	return func(checkID string) ([]audit.Finding, error) {
		out := make([]audit.Finding, 0, n)
		for i := 0; i < n; i++ {
			sizeGB := 500 - i*10
			region := regions[i%len(regions)]
			perGB := 0.0960
			usd := float64(sizeGB) * perGB
			out = append(out, audit.Finding{
				Fingerprint: fmt.Sprintf("a1b2c3d%d", i), CheckID: checkID, Service: "EBS",
				Category: audit.CategoryWaste, Account: "548846591939",
				Region:     region,
				ResourceID: fmt.Sprintf("vol-0f066ace215f73%03d", i),
				Title:      fmt.Sprintf("vol-0f066ace215f73%03d · %d GB gp3 · unattached", i, sizeGB),
				Detail:     map[string]any{"size_gb": sizeGB, "volume_type": "gp3", "age_days": 62 - i},
				Magnitude: audit.Magnitude{
					Kind: audit.MagnitudeSavings, Basis: audit.BasisExact, USDPerMonth: usd,
				},
				Evidence: []string{
					fmt.Sprintf("%d GB × $%.4f/GB-month (gp3, %s)", sizeGB, perGB, region),
					fmt.Sprintf("= $%.2f per month", usd),
				},
			})
		}
		return out, nil
	}
}

// budgetRunner returns breached-budget fixtures: one ordinary monthly cost
// budget, one with no forecast yet, and one measured in GB rather than dollars.
func budgetRunner() tuiaudit.RunFunc {
	mk := func(name, period string, limit, actual float64, unit string, forecast *float64) audit.Finding {
		d := map[string]any{
			"limit": limit, "actual": actual, "over": actual - limit,
			"unit": unit, "period": period, "budget_type": "COST",
		}
		if forecast != nil {
			d["forecast"] = *forecast
		}
		return audit.Finding{
			CheckID: "budget-breached", Service: "Budgets", Category: audit.CategoryRisk,
			Account: "548846591939", Region: "global", ResourceID: name,
			Magnitude: audit.Magnitude{Kind: audit.MagnitudeCount, Basis: audit.BasisNone, Count: 1},
			Detail:    d,
			// Matches what engine.BudgetsBreached emits, thousands separators
			// and all — a fixture that formats differently from the real thing
			// is a mockup of a screen that does not exist.
			Evidence: []string{
				fmt.Sprintf("actual %s over a %s limit of %s", amt(actual, unit), period, amt(limit, unit)),
				fmt.Sprintf("= %s over, %.0f%% of limit", amt(actual-limit, unit), actual/limit*100),
			},
		}
	}
	f1, f2 := 15400.0, 980.0
	return func(string) ([]audit.Finding, error) {
		return []audit.Finding{
			mk("platform-monthly", "monthly", 12000, 13204.55, "USD", &f1),
			mk("data-eng-monthly", "monthly", 800, 921.40, "USD", &f2),
			mk("sandbox-daily", "daily", 50, 61.20, "USD", nil),
			mk("s3-storage-usage", "monthly", 500, 512.75, "GB", nil),
		}, nil
	}
}

// amt mirrors the engine's amount() so fixtures read like real findings.
func amt(v float64, unit string) string {
	whole := fmt.Sprintf("%.2f", v)
	dot := strings.IndexByte(whole, '.')
	var out []byte
	for i := 0; i < dot; i++ {
		if i > 0 && (dot-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, whole[i])
	}
	s := string(out) + whole[dot:]
	if unit == "USD" {
		return "$" + s
	}
	return s + " " + unit
}

// viewAfter drives a model through startup plus the given keys, running any
// commands they produce, then returns the frame.
func viewAfter(m tea.Model, keys ...tea.Msg) string {
	m, _ = m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m, _ = m.Update(tea.BackgroundColorMsg{Color: nil})
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = m.Update(k)
		// Run the command synchronously so the scan result lands before render.
		for cmd != nil {
			msg := cmd()
			m, cmd = m.Update(msg)
		}
	}
	return m.View().Content
}

// view drives a model through the messages a real program would send at
// startup, then returns the frame it would draw.
var termW, termH = 80, 24

func view(m tea.Model) string {
	m, _ = m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	m, _ = m.Update(tea.BackgroundColorMsg{Color: nil})
	return m.View().Content
}

func laptopWithExpiredSSO() core.Discovery {
	expired := time.Now().Add(-42 * time.Minute)
	return core.Discovery{
		Profiles: []string{"dev", "prod", "stage"},
		Sessions: []string{"acme-sso"},
		Sources: []core.Source{
			{Kind: core.KindSSO, Name: "acme-sso", Detail: `session "acme-sso"`,
				Available: true, Note: "expired 42m ago", Expiry: expired},
			{Kind: core.KindProfile, Detail: "3 found", Available: true},
			{Kind: core.KindEnv, Detail: "environment variables", Note: "not set"},
			{Kind: core.KindAmbient, Detail: "instance metadata", Note: "not on EC2/ECS"},
		},
	}
}

// afterARun is what the catalogue looks like once results exist: most checks
// run recently, one stale, one never run. Impact strings are pre-formatted
// because categories are not commensurable — savings, unattributed spend, and
// counts cannot share a column format.
func afterARun() map[string]tuiaudit.Result {
	return map[string]tuiaudit.Result{
		"ebs-unattached":         {Findings: 8, Impact: "$890/mo", Age: "26m ago", Value: 890},
		"ec2-stopped-ebs":        {Findings: 12, Impact: "$537/mo", Age: "26m ago", Value: 537},
		"ebs-orphaned-snapshots": {Findings: 23, Impact: "$312/mo", Age: "26m ago", Value: 312},
		"eip-unassociated":       {Findings: 3, Impact: "$108/mo", Age: "26m ago", Value: 108},
		"ebs-gp2-to-gp3":         {Findings: 23, Impact: "$314/mo", Age: "26m ago", Value: 314},
		"untagged-resources":     {Findings: 312, Impact: "$26,410/mo", Age: "3d ago", Stale: true, Value: 26410},
	}
}

func nothingConfigured() core.Discovery {
	return core.Discovery{Sources: []core.Source{
		{Kind: core.KindSSO, Detail: "IAM Identity Center", Note: "no sso-session configured"},
		{Kind: core.KindProfile, Detail: "shared profiles", Note: "none configured"},
		{Kind: core.KindEnv, Detail: "environment variables", Note: "not set"},
		{Kind: core.KindAmbient, Detail: "instance metadata", Note: "not on EC2/ECS"},
	}}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
