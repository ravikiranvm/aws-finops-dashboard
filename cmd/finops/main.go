// Command finops is a terminal-native AWS cost and resource audit tool.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/exitcode"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/render"
)

var version = "3.0.0-dev"

func main() { os.Exit(run()) }

func run() int {
	var (
		format = flag.String("format", "",
			"output format: plain or json (default: interactive at a terminal, plain when piped)")
		out         = flag.String("out", "", "write output to a file instead of stdout")
		profile     = flag.String("profile", "", "AWS profile to use")
		region      = flag.String("region", "", "AWS region")
		noProbe     = flag.Bool("no-probe", false, "skip the IAM permission preflight")
		regions     = flag.String("regions", "", "comma-separated regions to scan (default: auto-detect)")
		only        = flag.String("check", "", "run a single check by id")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = usage
	flag.Parse()

	// Go's flag package stops at the first non-flag argument, so a flag
	// written after the subcommand — `finops auth --no-probe`, which is how
	// everyone writes it — would silently be treated as a positional and
	// ignored. Pull out the subcommand and parse what follows it.
	cmd := flag.Arg(0)
	if cmd != "" {
		if err := flag.CommandLine.Parse(flag.Args()[1:]); err != nil {
			return exitcode.Usage
		}
		if extra := flag.Arg(0); extra != "" {
			fmt.Fprintf(os.Stderr, "finops: unexpected argument %q\n\n", extra)
			usage()
			return exitcode.Usage
		}
	}

	if *showVersion {
		fmt.Println(version)
		return exitcode.OK
	}

	mode, err := render.Resolve(render.Request{
		Format: *format,
		Out:    *out,
		IsTTY:  term.IsTerminal(os.Stdout.Fd()),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "finops: %v\n", err)
		return exitcode.Usage
	}

	// Ctrl-C should exit promptly and cleanly, including mid-API-call.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	opts := core.Options{Profile: *profile, Region: *region}

	var code int
	switch cmd {
	case "auth":
		code, err = runAuth(ctx, os.Stdout, mode, opts, *noProbe)
	case "audit":
		var rs []string
		if *regions != "" {
			for _, r := range strings.Split(*regions, ",") {
				if r = strings.TrimSpace(r); r != "" {
					rs = append(rs, r)
				}
			}
		}
		code, err = runAudit(ctx, mode, opts, rs, *only)
	case "":
		code, err = runRoot(ctx, mode, opts)
	default:
		fmt.Fprintf(os.Stderr, "finops: unknown command %q\n\n", cmd)
		usage()
		return exitcode.Usage
	}

	if err != nil {
		return reportError(mode, err)
	}
	return code
}

// reportError renders a failure for whichever audience is listening: a human
// gets a road sign, an agent gets a structured object with a remediation field
// it can act on. Both get a meaningful exit code.
func reportError(mode render.Mode, err error) int {
	var fe *core.Error
	if !errors.As(err, &fe) {
		fmt.Fprintf(os.Stderr, "finops: %v\n", err)
		return exitcode.Error
	}

	if mode == render.ModeJSON {
		payload := map[string]any{"error": map[string]string{
			"code":        fe.Code,
			"message":     fe.Message,
			"remediation": fe.Remediation,
		}}
		enc := json.NewEncoder(os.Stderr)
		enc.SetIndent("", "  ")
		_ = enc.Encode(payload)
		return fe.Exit
	}

	fmt.Fprintf(os.Stderr, "✗ %s\n", fe.Message)
	if fe.Remediation != "" {
		fmt.Fprintf(os.Stderr, "\n  Fix:  %s\n", fe.Remediation)
	}
	return fe.Exit
}

func usage() {
	fmt.Fprintf(os.Stderr, `finops %s — AWS cost and waste audit

Usage:
  finops [flags]           open the audit view
  finops audit [flags]     run the audit checks
  finops auth [flags]      show identity, source, and permissions

Flags:
`, version)
	flag.PrintDefaults()
}
