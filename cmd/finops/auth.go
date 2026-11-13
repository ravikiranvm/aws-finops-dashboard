package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/exitcode"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/render"
)

// authReport is the serialisable shape of `finops auth`. It is also the JSON
// contract, so field names are stable once released.
type authReport struct {
	Identity struct {
		ARN       string `json:"arn"`
		AccountID string `json:"account_id"`
		Region    string `json:"region"`
		Source    string `json:"source"`
		SourceRef string `json:"source_ref,omitempty"`
		ExpiresAt string `json:"expires_at,omitempty"`
	} `json:"identity"`
	Organization *struct {
		ID           string `json:"id"`
		IsManagement bool   `json:"is_management"`
		AccountCount int    `json:"account_count"`
	} `json:"organization,omitempty"`
	Permissions []struct {
		Action  string `json:"action"`
		State   string `json:"state"`
		Enables string `json:"enables,omitempty"`
		Note    string `json:"note,omitempty"`
	} `json:"permissions"`
}

// runAuth implements `finops auth`: who am I, how did I get here, and what can
// I actually do. A one-shot fact, so it prints and exits rather than opening a
// view (OVERHAUL.md D-B4).
func runAuth(ctx context.Context, out io.Writer, mode render.Mode, opts core.Options, skipProbe bool) (int, error) {
	resolver := core.NewResolver(core.OSEnv{})

	id, cfg, err := resolver.Resolve(ctx, opts)
	if err != nil {
		return 0, err
	}

	org, orgErr := core.DescribeOrg(ctx, cfg, id)

	var pre core.Preflight
	if !skipProbe {
		pre = core.CheckPermissions(ctx, cfg, id.AccountID)
	}

	if mode == render.ModeJSON {
		return exitcode.OK, writeAuthJSON(out, id, org, pre)
	}
	writeAuthPlain(out, id, org, pre, orgErr, skipProbe)
	return exitcode.OK, nil
}

func writeAuthJSON(out io.Writer, id core.Identity, org core.Org, pre core.Preflight) error {
	var r authReport
	r.Identity.ARN = id.ARN
	r.Identity.AccountID = id.AccountID
	r.Identity.Region = id.Region
	r.Identity.Source = string(id.Source)
	r.Identity.SourceRef = id.SourceName
	if !id.Expiry.IsZero() {
		r.Identity.ExpiresAt = id.Expiry.UTC().Format(time.RFC3339)
	}

	if org.ID != "" {
		r.Organization = &struct {
			ID           string `json:"id"`
			IsManagement bool   `json:"is_management"`
			AccountCount int    `json:"account_count"`
		}{org.ID, org.IsManagement, len(org.Accounts)}
	}

	// Emit [] rather than null: a consumer iterating the field should not
	// have to special-case a missing list.
	r.Permissions = make([]struct {
		Action  string `json:"action"`
		State   string `json:"state"`
		Enables string `json:"enables,omitempty"`
		Note    string `json:"note,omitempty"`
	}, 0, len(pre.Permissions))
	for _, p := range pre.Permissions {
		r.Permissions = append(r.Permissions, struct {
			Action  string `json:"action"`
			State   string `json:"state"`
			Enables string `json:"enables,omitempty"`
			Note    string `json:"note,omitempty"`
		}{p.Action, string(p.State), p.Enables, p.Note})
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func writeAuthPlain(out io.Writer, id core.Identity, org core.Org, pre core.Preflight, orgErr error, skipProbe bool) {
	f := func(label, value string) { fmt.Fprintf(out, "%-10s %s\n", label, value) }

	f("Identity", id.ARN)

	account := id.AccountID
	switch {
	case org.IsManagement:
		account += "   (Organizations management)"
	case org.ID != "":
		account += "   (Organizations member)"
	}
	f("Account", account)

	f("Source", describeSource(id))
	f("Region", id.Region)

	switch {
	case org.IsManagement:
		f("Mode", fmt.Sprintf("organization · %d accounts", len(org.Accounts)))
	case org.ID != "":
		f("Mode", "single account · member of "+org.ID)
	default:
		f("Mode", "single account")
	}
	if orgErr != nil {
		f("", "⚠ organization partially discovered: "+orgErr.Error())
	}

	if skipProbe {
		fmt.Fprintf(out, "\nPermissions not checked (--no-probe)\n")
		return
	}

	fmt.Fprintf(out, "\nPermissions\n")
	for _, p := range pre.Permissions {
		glyph := "✓"
		suffix := ""
		switch p.State {
		case core.PermDenied:
			glyph, suffix = "✗", "  denied — "+p.Enables+" unavailable"
		case core.PermUnchecked:
			glyph, suffix = "·", "  "+p.Note
		case core.PermError:
			glyph, suffix = "?", "  could not verify"
		}
		fmt.Fprintf(out, "  %s %-30s%s\n", glyph, p.Action, suffix)
	}

	granted, total := pre.Counts()
	line := fmt.Sprintf("\n%d of %d permissions", granted, total)
	if degraded := pre.Degraded(); len(degraded) > 0 {
		line += " · " + strings.Join(degraded, ", ") + " unavailable"
	}
	fmt.Fprintln(out, line)
}

func describeSource(id core.Identity) string {
	switch id.Source {
	case core.KindSSO:
		s := fmt.Sprintf("IAM Identity Center · session %q", id.SourceName)
		if !id.Expiry.IsZero() {
			s += fmt.Sprintf(" · expires %s", id.Expiry.Local().Format("15:04"))
		}
		return s
	case core.KindProfile:
		return fmt.Sprintf("shared profile %q", id.SourceName)
	case core.KindEnv:
		return "environment variables"
	case core.KindOIDC:
		return "web identity (OIDC)"
	case core.KindAmbient:
		return "instance or container role"
	default:
		return "default credential chain"
	}
}
