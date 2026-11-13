package render

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want Mode
	}{
		// Inference when nothing is asked for.
		{"tty, nothing asked for", Request{IsTTY: true}, ModeTUI},
		{"not a tty", Request{IsTTY: false}, ModePlain},

		// An explicit format always wins, in both directions.
		{"json on a tty", Request{Format: "json", IsTTY: true}, ModeJSON},
		{"json when piped", Request{Format: "json"}, ModeJSON},
		{"plain on a tty", Request{Format: "plain", IsTTY: true}, ModePlain},

		// Sending output somewhere is a data request.
		{"out sink on a tty", Request{Out: "report.txt", IsTTY: true}, ModePlain},
		{"format beats out sink", Request{Format: "json", Out: "report.txt", IsTTY: true}, ModeJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.req)
			if err != nil {
				t.Fatalf("Resolve(%+v) unexpected error: %v", tt.req, err)
			}
			if got != tt.want {
				t.Errorf("Resolve(%+v) = %v, want %v", tt.req, got, tt.want)
			}
		})
	}
}

func TestResolveRejectsUnknownFormat(t *testing.T) {
	// A typo must fail loudly rather than silently producing something else.
	for _, bad := range []string{"yaml", "csv", "markdown", "tui", "auto", "JSON", "  json"} {
		t.Run(bad, func(t *testing.T) {
			_, err := Resolve(Request{Format: bad, IsTTY: true})
			if err == nil {
				t.Errorf("Resolve(format=%q) should have errored", bad)
			}
		})
	}
}

func TestModeProperties(t *testing.T) {
	// Only the TUI takes keyboard input.
	for _, m := range []Mode{ModePlain, ModeJSON} {
		if m.Interactive() {
			t.Errorf("%v should not be interactive", m)
		}
	}
	if !ModeTUI.Interactive() {
		t.Error("ModeTUI should be interactive")
	}

	// Only the interactive view is styled. Plain must stay plain, or piped
	// output writes escape codes into files.
	for _, m := range []Mode{ModePlain, ModeJSON} {
		if m.Styled() {
			t.Errorf("%v must not be styled", m)
		}
	}
	if !ModeTUI.Styled() {
		t.Error("ModeTUI should be styled")
	}
}
