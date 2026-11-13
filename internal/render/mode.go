// Package render decides and performs output. The most important decision it
// makes is whether we are talking to a human at a terminal or to a pipe.
package render

import (
	"fmt"
	"strings"
)

// Mode is how output should be produced for this invocation.
//
// There are exactly three, because the tool has exactly two users and one of
// them reads in two situations:
//
//	ModeTUI    human, at a terminal, exploring
//	ModePlain  human, piped or redirected — CI logs, files, grep
//	ModeJSON   agent — scripts, MCP, automation
type Mode int

const (
	// ModeTUI is the interactive, inline-rendered view. Only ever chosen when
	// stdout is a terminal and the caller did not ask for data.
	ModeTUI Mode = iota
	// ModePlain is the same content as the TUI rendered as a single frame:
	// no input, no colour, no spinners.
	ModePlain
	// ModeJSON is the machine contract.
	ModeJSON
)

func (m Mode) String() string {
	switch m {
	case ModeTUI:
		return "tui"
	case ModePlain:
		return "plain"
	case ModeJSON:
		return "json"
	}
	return "unknown"
}

// Interactive reports whether this mode accepts keyboard input.
func (m Mode) Interactive() bool { return m == ModeTUI }

// Styled reports whether ANSI escape sequences may be emitted.
//
// Only the interactive view is styled. "Plain" means plain text: it covers
// both --no-tui at a terminal and piped output, and colouring the first would
// mean the second writes escape codes into files.
func (m Mode) Styled() bool { return m == ModeTUI }

// Request is what the caller asked for, before we decide what they get.
type Request struct {
	// Format is the value of --format: "plain", "json", or "" to let the
	// tool decide. An explicit value always wins over inference.
	//
	// There is deliberately no "tui" value: if stdout is a terminal the
	// default already is the TUI, and if it isn't, forcing one produces
	// garbage. Adding a value later is easy; removing one is breaking.
	Format string
	// Out is the value of --out (a sink such as a file path), or "".
	// Sending output somewhere always implies a non-interactive render.
	Out string
	// IsTTY reports whether stdout is a terminal.
	IsTTY bool
}

// Formats are the accepted --format values, for flag help and validation.
// Unset means "decide for me".
var Formats = []string{"plain", "json"}

// Resolve decides the output mode, or reports an unusable --format value.
//
// The governing rule (see OVERHAUL.md G1): a TUI is for exploring, static
// output is for answering and piping. An explicit --format always wins over
// inference — asking for JSON and getting a TUI would break every pipe and
// every agent invocation.
func Resolve(r Request) (Mode, error) {
	switch r.Format {
	case "json":
		return ModeJSON, nil
	case "plain":
		return ModePlain, nil
	case "":
		// fall through to inference
	default:
		return ModeTUI, fmt.Errorf("unknown --format %q (want %s, or omit to decide automatically)",
			r.Format, strings.Join(Formats, " or "))
	}

	// Sending output somewhere is a data request, whatever the terminal says.
	if r.Out != "" {
		return ModePlain, nil
	}
	// Otherwise: interactive only when there is a human to be interactive with.
	if r.IsTTY {
		return ModeTUI, nil
	}
	return ModePlain, nil
}
