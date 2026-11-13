package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/engine"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/exitcode"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/render"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/store"
	tuiaudit "github.com/ravikiranvm/aws-finops-dashboard/internal/tui/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/auth"
)

// runRoot handles bare `finops`.
//
// Credentials resolve silently when they can, and the picker only appears when
// they cannot — auth is a state shown in the header, not a toll booth to pass
// through on every launch (OVERHAUL.md D-B4).
func runRoot(ctx context.Context, mode render.Mode, opts core.Options) (int, error) {
	resolver := core.NewResolver(core.OSEnv{})

	id, cfg, err := resolver.Resolve(ctx, opts)
	if err == nil {
		org, _ := core.DescribeOrg(ctx, cfg, id)

		// The runner is injected so the TUI never imports AWS: it knows how to
		// ask for a scan, not how to perform one.
		runner := func(checkID string) ([]audit.Finding, error) {
			check, ok := audit.Get(checkID)
			if !ok {
				return nil, fmt.Errorf("unknown check %q", checkID)
			}
			scope, derr := engine.DetectRegions(ctx, cfg)
			if derr != nil {
				return nil, derr
			}
			res := engine.Run(ctx, engine.Request{
				Targets: []engine.Target{{Account: core.Account{ID: id.AccountID}, Config: cfg}},
				Regions: scope.Regions,
				Checks:  []audit.Check{check},
				Pricer:  engine.NewAWSPricer(cfg),
			})
			return res.Findings, nil
		}
		return showCatalog(ctx, mode, id.Summary(org), id.AccountID, runner)
	}

	// Outside a terminal there is nobody to ask, so fail with the road sign.
	if mode != render.ModeTUI {
		return 0, err
	}

	var fe *core.Error
	if !errors.As(err, &fe) || (fe.Code != core.CodeNoCredentials && fe.Code != core.CodeAuthExpired) {
		return 0, err
	}

	choice, perr := runPicker(ctx)
	if perr != nil {
		return 0, perr
	}
	if choice.Cancelled {
		// Quitting the picker is a choice, not a failure — but there are still
		// no usable credentials, so the exit code has to say so.
		return fe.Exit, nil
	}
	return applyChoice(ctx, choice, opts)
}

// showCatalog opens the audit catalogue — the tool's home. Results are empty
// until the audit engine lands, which is the honest first-run state: every
// check listed, none run.
func showCatalog(ctx context.Context, mode render.Mode, identity, account string, runner tuiaudit.RunFunc) (int, error) {
	st := store.New()
	results := cachedResults(st.Load(account))

	switch mode {
	case render.ModeJSON:
		return exitcode.OK, writeCatalogJSON(os.Stdout, results)
	case render.ModePlain:
		writeCatalogPlain(os.Stdout, results)
		return exitcode.OK, nil
	}

	m := tuiaudit.NewCatalog(results, version, identity, runner).
		SetPersist(func(checkID string, findings []audit.Finding) {
			var total float64
			priced := false
			for _, f := range findings {
				if f.Magnitude.Priced() {
					total += f.Magnitude.USDPerMonth
					priced = true
				}
			}
			// Failing to remember is a degradation, not a failure: the scan
			// already happened and is on screen.
			_ = st.Record(account, store.Run{
				CheckID: checkID, Findings: len(findings),
				USDPerMonth: total, Priced: priced,
			})
		})
	if _, err := tea.NewProgram(m, tea.WithContext(ctx)).Run(); err != nil {
		return 0, fmt.Errorf("audit catalogue: %w", err)
	}
	return exitcode.OK, nil
}

// writeCatalogPlain lists the catalogue as text for pipes and CI logs.
func writeCatalogPlain(out io.Writer, results map[string]tuiaudit.Result) {
	byCat := audit.ByCategory()
	for _, cat := range audit.Order {
		checks := byCat[cat]
		if len(checks) == 0 {
			continue
		}
		fmt.Fprintf(out, "%s\n", strings.ToUpper(cat.Title()))
		for _, c := range checks {
			state := "never run"
			if r, ok := results[c.ID]; ok {
				state = fmt.Sprintf("%d findings · %s · %s", r.Findings, r.Impact, r.Age)
			}
			fmt.Fprintf(out, "  %-34s %s\n", c.Name, state)
		}
	}
}

// writeCatalogJSON is the agent view of what this tool can check.
func writeCatalogJSON(out io.Writer, results map[string]tuiaudit.Result) error {
	type entry struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Service     string   `json:"service"`
		Category    string   `json:"category"`
		Basis       string   `json:"basis"`
		Magnitude   string   `json:"magnitude"`
		Description string   `json:"description"`
		APIs        []string `json:"apis"`
		Findings    *int     `json:"findings,omitempty"`
		Impact      string   `json:"impact,omitempty"`
		LastRun     string   `json:"last_run,omitempty"`
	}

	checks := audit.All()
	entries := make([]entry, 0, len(checks))
	for _, c := range checks {
		e := entry{
			ID: c.ID, Name: c.Name, Service: c.Service,
			Category: string(c.Category), Basis: string(c.Basis),
			Magnitude: string(c.Magnitude), Description: c.Description, APIs: c.APIs,
		}
		if r, ok := results[c.ID]; ok {
			f := r.Findings
			e.Findings, e.Impact, e.LastRun = &f, r.Impact, r.Age
		}
		entries = append(entries, e)
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"audits": entries})
}

func runPicker(ctx context.Context) (auth.Choice, error) {
	d := core.Discover(core.OSEnv{})

	// Alt-screen is declared by the view itself in Bubble Tea v2, not by a
	// program option — see the picker's View and docs/STACK.md.
	final, err := tea.NewProgram(auth.NewPicker(d, version), tea.WithContext(ctx)).Run()
	if err != nil {
		return auth.Choice{}, fmt.Errorf("credential picker: %w", err)
	}
	m, ok := final.(auth.Model)
	if !ok {
		return auth.Choice{}, fmt.Errorf("credential picker returned an unexpected model")
	}
	return m.Choice(), nil
}

// applyChoice acts on the picker's selection. The TUI decides; execution
// happens here, after the program has released the terminal — otherwise
// `aws sso login` would fight Bubble Tea for stdin.
func applyChoice(ctx context.Context, c auth.Choice, opts core.Options) (int, error) {
	switch c.Kind {
	case core.KindSSO:
		fmt.Printf("Running: aws sso login --sso-session %s\n\n", c.Name)
		cmd := exec.CommandContext(ctx, "aws", "sso", "login", "--sso-session", c.Name)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return 0, &core.Error{
				Code:        core.CodeAuthExpired,
				Message:     fmt.Sprintf("aws sso login failed for session %q", c.Name),
				Remediation: fmt.Sprintf("run it directly to see why: aws sso login --sso-session %s", c.Name),
				Exit:        3,
				Err:         err,
			}
		}
	case core.KindProfile:
		opts.Profile = c.Name
	}

	id, _, err := core.NewResolver(core.OSEnv{}).Resolve(ctx, opts)
	if err != nil {
		return 0, err
	}
	fmt.Printf("\n%s %s\n", "✓", id.Summary(core.Org{}))
	fmt.Printf("\nTry: finops auth\n")
	return exitcode.OK, nil
}

// cachedResults turns a persisted cache into catalogue rows, deriving the age
// and staleness from real timestamps rather than the "just now" a live scan
// produces.
func cachedResults(c store.Cache) map[string]tuiaudit.Result {
	out := map[string]tuiaudit.Result{}
	for id, run := range c.Runs {
		r := tuiaudit.Result{
			Findings: run.Findings,
			Age:      humanAge(run.Age()),
			Stale:    run.Stale(),
			Value:    run.USDPerMonth,
		}
		switch {
		case run.Findings == 0:
			r.Impact = "none"
		case !run.Priced:
			r.Impact = "unpriced"
		default:
			r.Impact = tuiaudit.Money(run.USDPerMonth) + "/mo"
		}
		out[id] = r
	}
	return out
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
