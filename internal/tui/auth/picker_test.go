package auth

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
)

// fixtureDiscovery is a laptop with an expired SSO session and three profiles:
// the case the picker exists to handle.
func fixtureDiscovery() core.Discovery {
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

// newTest starts the picker at a fixed size with a known background, so frames
// are deterministic rather than depending on the terminal running the tests.
func newTest(t *testing.T, d core.Discovery) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, NewPicker(d, "3.0"), teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.BackgroundColorMsg{Color: nil}) // light background, deterministic
	return tm
}

func finalFrame(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPickerInitialFrame(t *testing.T) {
	tm := newTest(t, fixtureDiscovery())
	tm.Send(tea.KeyPressMsg{Code: 'q'})
	golden.RequireEqual(t, finalFrame(t, tm))
}

func TestPickerMovesSelection(t *testing.T) {
	tm := newTest(t, fixtureDiscovery())
	tm.Send(tea.KeyPressMsg{Code: 'j'})
	tm.Send(tea.KeyPressMsg{Code: 'j'})
	tm.Send(tea.KeyPressMsg{Code: 'q'})
	golden.RequireEqual(t, finalFrame(t, tm))
}

func TestPickerNoCredentialsAtAll(t *testing.T) {
	// The empty case is a setup guide, not an empty list: a user with nothing
	// configured needs the command that fixes it.
	tm := newTest(t, core.Discovery{Sources: []core.Source{
		{Kind: core.KindSSO, Detail: "IAM Identity Center", Note: "no sso-session configured"},
		{Kind: core.KindProfile, Detail: "shared profiles", Note: "none configured"},
		{Kind: core.KindEnv, Detail: "environment variables", Note: "not set"},
		{Kind: core.KindAmbient, Detail: "instance metadata", Note: "not on EC2/ECS"},
	}})
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	out := finalFrame(t, tm)
	if !bytes.Contains(out, []byte("aws configure sso")) {
		t.Error("empty state must tell the user how to set credentials up")
	}
	golden.RequireEqual(t, out)
}

func TestPickerSelectionResult(t *testing.T) {
	tests := []struct {
		name     string
		presses  []rune
		wantKind core.Kind
		wantName string
	}{
		{"first option is the sso sign-in", nil, core.KindSSO, "acme-sso"},
		{"second option is the first profile", []rune{'j'}, core.KindProfile, "dev"},
		{"third option is the second profile", []rune{'j', 'j'}, core.KindProfile, "prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := newTest(t, fixtureDiscovery())
			for _, r := range tt.presses {
				tm.Send(tea.KeyPressMsg{Code: r})
			}
			tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

			m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
			if !ok {
				t.Fatal("final model was not auth.Model")
			}
			got := m.Choice()
			if got.Cancelled {
				t.Fatal("selection should not be cancelled")
			}
			if got.Kind != tt.wantKind || got.Name != tt.wantName {
				t.Errorf("choice = %v/%q, want %v/%q", got.Kind, got.Name, tt.wantKind, tt.wantName)
			}
		})
	}
}

func TestPickerQuitIsCancelled(t *testing.T) {
	tm := newTest(t, fixtureDiscovery())
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	m := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
	if !m.Choice().Cancelled {
		t.Error("quitting must report Cancelled so the caller does not treat it as a selection")
	}
}
