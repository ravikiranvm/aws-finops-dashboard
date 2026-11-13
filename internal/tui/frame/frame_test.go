package frame

import (
	"strings"
	"testing"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/theme"
)

// TestRenderFillsExactHeight pins the relationship between chromeHeight and
// what Render actually emits. They are separate pieces of arithmetic over the
// same layout, and when they disagree the frame runs past the bottom of the
// terminal — which in the alternate screen scrolls the top rule out of sight
// rather than failing visibly.
func TestRenderFillsExactHeight(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Frame)
	}{
		{"plain", func(f *Frame) {}},
		{"with command", func(f *Frame) { f.Command = "→ finops audit --check ebs-unattached" }},
		{"with one-line detail", func(f *Frame) { f.Detail = "Unattached EBS volumes · Waste" }},
		{"with two-line detail", func(f *Frame) { f.Detail = "Unattached EBS volumes · Waste\nVolumes in the available state." }},
		{"with four-line detail", func(f *Frame) { f.Detail = "a\nb\nc\nd" }},
		{"detail and command", func(f *Frame) { f.Detail = "a\nb"; f.Command = "→ finops audit" }},
	}

	for _, style := range []BorderStyle{BorderRules, BorderFull, BorderNone} {
		for _, tc := range cases {
			for _, h := range []int{24, 30, 50} {
				f := Frame{
					Theme: theme.New(true), Width: 80, Height: h,
					Tool: "finops 3.0", Breadcrumb: "audit", Keys: "q quit",
					Style: style,
				}
				tc.mutate(&f)

				// Deliberately short content, so any discrepancy shows up as the
				// frame being the wrong height rather than as clipped content.
				got := len(strings.Split(f.Render("one line"), "\n"))
				if got != h {
					t.Errorf("style=%d %s h=%d: rendered %d rows, want %d",
						style, tc.name, h, got, h)
				}
			}
		}
	}
}
