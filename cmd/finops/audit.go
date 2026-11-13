package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/engine"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/exitcode"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/render"
)

// runAudit executes the enabled checks and reports what it found.
//
// Free to run: no Cost Explorer calls anywhere in this path. That is what makes
// it safe as the tool's default action (OVERHAUL.md G5).
func runAudit(ctx context.Context, mode render.Mode, opts core.Options, regionsFlag []string, only string) (int, error) {
	id, cfg, err := core.NewResolver(core.OSEnv{}).Resolve(ctx, opts)
	if err != nil {
		return 0, err
	}

	regions := regionsFlag
	regionSource := "flag"
	if len(regions) == 0 {
		scope, derr := engine.DetectRegions(ctx, cfg)
		if derr != nil {
			return 0, derr
		}
		regions, regionSource = scope.Regions, scope.Source
	}

	checks := audit.All()
	if only != "" {
		c, ok := audit.Get(only)
		if !ok {
			return exitcode.Usage, fmt.Errorf("unknown check %q — run 'finops' to list them", only)
		}
		checks = []audit.Check{c}
	}

	res := engine.Run(ctx, engine.Request{
		Targets: []engine.Target{{
			Account: core.Account{ID: id.AccountID},
			Config:  cfg,
		}},
		Regions: regions,
		Checks:  checks,
		Pricer:  engine.NewAWSPricer(cfg),
	})

	if mode == render.ModeJSON {
		if err := writeAuditJSON(os.Stdout, id, regions, res); err != nil {
			return 0, err
		}
	} else {
		writeAuditPlain(os.Stdout, id, regions, regionSource, res)
	}

	// Exit 10 when findings exist, so the audit works as a CI guardrail without
	// anyone parsing output (OVERHAUL.md R-C29).
	if len(res.Findings) > 0 {
		return exitcode.Findings, nil
	}
	return exitcode.OK, nil
}

func writeAuditPlain(out io.Writer, id core.Identity, regions []string, regionSource string, res engine.Result) {
	fmt.Fprintf(out, "FinOps Audit · account %s · %d region(s) %v (%s)\n",
		id.AccountID, len(regions), regions, regionSource)

	usd, priced, unpriced := res.TotalSavings()
	fmt.Fprintf(out, "$%.2f/month addressable · %d findings", usd, len(res.Findings))
	if unpriced > 0 {
		// Never let an unpriced finding be silently counted as zero.
		fmt.Fprintf(out, " (%d priced, %d without a price)", priced, unpriced)
	}
	fmt.Fprintf(out, " · %s\n\n", res.Duration.Round(1e7))

	if len(res.Findings) == 0 {
		fmt.Fprintf(out, "Nothing found.\n")
	} else {
		byCategory := map[audit.Category][]audit.Finding{}
		for _, f := range res.Findings {
			byCategory[f.Category] = append(byCategory[f.Category], f)
		}
		for _, cat := range audit.Order {
			fs := byCategory[cat]
			if len(fs) == 0 {
				continue
			}
			// Money at the top: an operator reads down and stops when the
			// numbers stop mattering.
			sort.Slice(fs, func(i, j int) bool {
				return fs[i].Magnitude.USDPerMonth > fs[j].Magnitude.USDPerMonth
			})
			fmt.Fprintf(out, "%s\n", cat.Title())
			for _, f := range fs {
				// "/mo" only where a monthly figure exists. A count-carrying
				// finding printed as "—/mo" claims a rate it never had, and
				// reads as a pricing failure rather than as a check that was
				// never about money.
				lead := "          "
				switch {
				case f.Magnitude.Priced():
					lead = fmt.Sprintf("$%6.2f/mo", f.Magnitude.USDPerMonth)
				case f.Magnitude.Kind == audit.MagnitudeCount:
					lead = "         !"
				default:
					lead = "     —/mo"
				}
				fmt.Fprintf(out, "  %s  %s\n", lead, f.Title)
				for _, e := range f.Evidence {
					fmt.Fprintf(out, "              %s\n", e)
				}
			}
			fmt.Fprintln(out)
		}
	}

	// Coverage and skips are part of the result, not a footnote: a total that
	// silently excluded two regions is misleading.
	if !res.Coverage.Complete() {
		fmt.Fprintf(out, "⚠ %s\n", res.Coverage.Summary())
	}
	for _, cid := range res.Coverage.SkippedIDs() {
		fmt.Fprintf(out, "· skipped %s — %s\n", cid, res.Coverage.ChecksSkipped[cid])
	}
	for _, f := range res.Failures {
		fmt.Fprintf(out, "⚠ %s/%s %s: %s\n", f.Account, f.Region, f.CheckID, f.Message)
	}
}

func writeAuditJSON(out io.Writer, id core.Identity, regions []string, res engine.Result) error {
	usd, priced, unpriced := res.TotalSavings()

	type finding struct {
		Fingerprint string         `json:"fingerprint"`
		CheckID     string         `json:"check_id"`
		Service     string         `json:"service"`
		Category    string         `json:"category"`
		Account     string         `json:"account"`
		Region      string         `json:"region"`
		ResourceID  string         `json:"resource_id"`
		Title       string         `json:"title"`
		Owner       string         `json:"owner,omitempty"`
		Basis       string         `json:"basis"`
		USDPerMonth *float64       `json:"usd_per_month,omitempty"`
		Detail      map[string]any `json:"detail,omitempty"`
		Evidence    []string       `json:"evidence,omitempty"`
	}

	findings := make([]finding, 0, len(res.Findings))
	for _, f := range res.Findings {
		e := finding{
			Fingerprint: f.Fingerprint, CheckID: f.CheckID, Service: f.Service,
			Category: string(f.Category), Account: f.Account, Region: f.Region,
			ResourceID: f.ResourceID, Title: f.Title, Owner: f.Owner,
			Basis: string(f.Magnitude.Basis), Detail: f.Detail, Evidence: f.Evidence,
		}
		// Omitted rather than zero when unpriced: an agent must be able to tell
		// "no saving" from "we could not price it".
		if f.Magnitude.Priced() {
			v := f.Magnitude.USDPerMonth
			e.USDPerMonth = &v
		}
		findings = append(findings, e)
	}

	payload := map[string]any{
		"schema_version": "3.0",
		"account":        id.AccountID,
		"regions":        regions,
		"summary": map[string]any{
			"findings":              len(res.Findings),
			"savings_usd_per_month": usd,
			"priced_findings":       priced,
			"unpriced_findings":     unpriced,
			"duration_ms":           res.Duration.Milliseconds(),
		},
		"coverage": map[string]any{
			"complete":         res.Coverage.Complete(),
			"accounts_scanned": res.Coverage.AccountsScanned,
			"regions_scanned":  res.Coverage.RegionsScanned,
			"checks_run":       res.Coverage.ChecksRun,
			"checks_skipped":   res.Coverage.ChecksSkipped,
		},
		"findings": findings,
		"failures": res.Failures,
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
