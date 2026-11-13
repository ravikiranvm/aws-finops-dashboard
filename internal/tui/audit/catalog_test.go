package audit

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/exp/golden"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
)

func withResults() map[string]Result {
	return map[string]Result{
		"ebs-unattached":         {Findings: 8, Impact: "$890/mo", Age: "26m ago", Value: 890},
		"ec2-stopped-ebs":        {Findings: 12, Impact: "$537/mo", Age: "26m ago", Value: 537},
		"ebs-orphaned-snapshots": {Findings: 23, Impact: "$312/mo", Age: "26m ago", Value: 312},
		"eip-unassociated":       {Findings: 3, Impact: "$108/mo", Age: "26m ago", Value: 108},
		"ebs-gp2-to-gp3":         {Findings: 23, Impact: "$314/mo", Age: "26m ago", Value: 314},
		"untagged-resources":     {Findings: 312, Impact: "$26,410/mo", Age: "3d ago", Stale: true, Value: 26410},
	}
}

func newTest(t *testing.T, results map[string]Result) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, NewCatalog(results, "3.0", "prod-mgmt · org 14", nil),
		teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.BackgroundColorMsg{Color: nil})
	return tm
}

// finalFrame renders the model's last state directly, and is what golden
// frames are taken from.
//
// Not the same thing as reading FinalOutput. The v2 renderer emits incremental,
// cursor-positioned updates, so the captured stream is a history of edits, not
// a picture: rows that have since scrolled away are still in it, and how the
// edits are coalesced depends on timing, which makes a snapshot of it flaky.
// Rendering the final model is deterministic and is the actual frame a person
// would see (docs/STACK.md).
func finalFrame(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
	if !ok {
		t.Fatal("final model is not an audit Model")
	}
	return m.View().Content
}

func frameOf(t *testing.T, tm *teatest.TestModel) []byte {
	t.Helper()
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCatalogNeverRun(t *testing.T) {
	tm := newTest(t, nil)
	tm.Send(tea.KeyPressMsg{Code: 'q'})
	out := frameOf(t, tm)

	// The first-run state must state coverage, not invent a total over an
	// empty set — a "$0 waste" headline would imply we had looked.
	if !bytes.Contains(out, []byte("No audits run yet")) {
		t.Error("first-run state should say nothing has run")
	}
	golden.RequireEqual(t, []byte(finalFrame(t, tm)))
}

func TestCatalogWithResults(t *testing.T) {
	tm := newTest(t, withResults())
	tm.Send(tea.KeyPressMsg{Code: 'q'})
	golden.RequireEqual(t, []byte(finalFrame(t, tm)))
}

func TestCatalogCollapse(t *testing.T) {
	tm := newTest(t, withResults())
	tm.Send(tea.KeyPressMsg{Code: ' '}) // cursor starts on the WASTE header
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	out := frameOf(t, tm)
	if bytes.Contains(out, []byte("Unattached EBS volumes")) {
		t.Error("collapsing WASTE should hide its checks")
	}
	golden.RequireEqual(t, []byte(finalFrame(t, tm)))
}

func TestHeaderCountOnlyWhenCollapsed(t *testing.T) {
	tm := newTest(t, withResults())
	tm.Send(tea.KeyPressMsg{Code: ' '}) // collapse WASTE, cursor starts there
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	frame := finalFrame(t, tm)
	if !strings.Contains(frame, "4 audits") {
		t.Error("a collapsed category should say how many it hides")
	}
	// RATE is still open, so its count is visible in the rows below it.
	if strings.Contains(frame, "1 audit ") || strings.Contains(frame, "1 audit\n") {
		t.Error("an expanded category should not repeat its count")
	}
}

func TestCatalogOrdersBySavings(t *testing.T) {
	m := NewCatalog(withResults(), "3.0", "", nil)

	// Within a category the money must be at the top: an operator reads down
	// and stops when the numbers stop mattering.
	var wasteOrder []string
	for _, r := range m.rows {
		if !r.header && r.check.Category == "waste" {
			wasteOrder = append(wasteOrder, r.check.ID)
		}
	}
	want := []string{"ebs-unattached", "ec2-stopped-ebs", "ebs-orphaned-snapshots", "eip-unassociated"}
	if len(wasteOrder) != len(want) {
		t.Fatalf("waste checks = %v, want %v", wasteOrder, want)
	}
	for i := range want {
		if wasteOrder[i] != want[i] {
			t.Errorf("waste[%d] = %q, want %q", i, wasteOrder[i], want[i])
		}
	}
}

func TestCatalogNeverRunSortsLast(t *testing.T) {
	// One waste check has no result; it should fall to the bottom of its
	// category rather than jumping the queue with an implicit zero.
	partial := map[string]Result{
		"eip-unassociated": {Findings: 3, Impact: "$108/mo", Age: "1m ago", Value: 108},
	}
	m := NewCatalog(partial, "3.0", "", nil)

	var first string
	for _, r := range m.rows {
		if !r.header && r.check.Category == "waste" {
			first = r.check.ID
			break
		}
	}
	if first != "eip-unassociated" {
		t.Errorf("first waste check = %q, want the only one with a result", first)
	}
}

func TestCatalogDisabledKeysAreHidden(t *testing.T) {
	tm := newTest(t, nil)
	tm.Send(tea.KeyPressMsg{Code: 'q'})
	out := frameOf(t, tm)

	// Running is not implemented, so those keys are disabled and must not be
	// advertised. The footer promising what the tool cannot do is worse than
	// a short footer.
	for _, promise := range []string{"run all", "settings", "results"} {
		if bytes.Contains(out, []byte(promise)) {
			t.Errorf("footer advertises %q, but the key is disabled", promise)
		}
	}
}

// TestCatalogColsFillWidth pins the arithmetic that ties catalogCols to
// gridSpan. They are computed in two places — the columns here, the span used
// by the header rule and category rows — and a mismatch shows up as a table
// that stops short of the frame, which reads as a rendering bug.
func TestCatalogColsFillWidth(t *testing.T) {
	for inner := 30; inner <= 300; inner++ {
		name, found, age, impact := catalogCols(inner)

		span := rowIndent + name + 1 + found + 1 + impact
		if age > 0 {
			span += age + 2
		}

		want := inner
		if want > maxGrid {
			want = maxGrid
		}
		if span < want {
			t.Errorf("inner=%d: grid spans %d, leaving %d columns unused", inner, span, want-span)
		}
		// Overflow is only tolerable once the name column has hit its floor: an
		// audit name below ~24 columns is unreadable, so past that point the
		// frame clips rather than the table shrinking into nonsense.
		if span > want && name > minName {
			t.Errorf("inner=%d: grid spans %d (name=%d), overflowing by %d", inner, span, name, span-want)
		}
	}
}

// TestLayoutFillsWidth is the findings table's version of the catalogue width
// invariant: the declared columns must add up to the width the header and rows
// are assembled at, or the table stops short of the frame.
//
// Run against every check that declares columns, so a new check cannot ship a
// declaration whose widths do not add up.
func TestLayoutFillsWidth(t *testing.T) {
	for _, check := range audit.All() {
		if len(check.Columns) == 0 {
			continue
		}
		for inner := 40; inner <= 300; inner++ {
			cols := layout(check.Columns, inner)

			span := rowIndent + len(cols) - 1
			flex := 0
			for _, c := range cols {
				span += c.Width
				if c.Flex {
					flex++
				}
			}
			if flex != 1 {
				t.Fatalf("%s inner=%d: %d flexible columns survived, want 1", check.ID, inner, flex)
			}

			want := inner
			if want > maxGrid {
				want = maxGrid
			}
			if span < want {
				t.Errorf("%s inner=%d: table spans %d, leaving %d columns unused",
					check.ID, inner, span, want-span)
			}
			// Overflow is only allowed once the flexible column has bottomed
			// out and there is nothing left to shed.
			flexW := 0
			for _, c := range cols {
				if c.Flex {
					flexW = c.Width
				}
			}
			if span > want && flexW > flexFloor {
				t.Errorf("%s inner=%d: table spans %d (flex=%d), overflowing by %d",
					check.ID, inner, span, flexW, span-want)
			}
		}
	}
}

// stubFindings builds n fixture findings for the results screen.
func stubFindings(n int) RunFunc {
	return func(checkID string) ([]audit.Finding, error) {
		out := make([]audit.Finding, 0, n)
		for i := 0; i < n; i++ {
			sizeGB := 500 - i*10
			usd := float64(sizeGB) * 0.096
			out = append(out, audit.Finding{
				CheckID: checkID, Service: "EBS", Category: audit.CategoryWaste,
				Account: "548846591939", Region: "ap-southeast-1",
				ResourceID: fmt.Sprintf("vol-0f066ace215f73%03d", i),
				Detail:     map[string]any{"size_gb": sizeGB, "volume_type": "gp3", "age_days": 62 - i},
				Magnitude: audit.Magnitude{
					Kind: audit.MagnitudeSavings, Basis: audit.BasisExact, USDPerMonth: usd,
				},
				Evidence: []string{
					fmt.Sprintf("%d GB × $0.0960/GB-month (gp3, ap-southeast-1)", sizeGB),
					fmt.Sprintf("= $%.2f per month", usd),
				},
			})
		}
		return out, nil
	}
}

func resultsTest(t *testing.T, n int) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, NewCatalog(nil, "3.0", "prod-mgmt · org 14", stubFindings(n)),
		teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.BackgroundColorMsg{Color: nil})
	// Two moves land on Unattached EBS volumes; 'r' runs it.
	tm.Send(tea.KeyPressMsg{Code: 'j'})
	tm.Send(tea.KeyPressMsg{Code: 'j'})
	tm.Send(tea.KeyPressMsg{Code: 'r'})

	// The scan completes asynchronously. Without waiting, keys meant for the
	// results screen can land while the running screen still has the focus,
	// where they are deliberately ignored — an intermittent failure that looks
	// like a navigation bug.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("exact basis"))
	}, teatest.WithDuration(3*time.Second))
	return tm
}

func TestResultsStripFollowsCursor(t *testing.T) {
	tm := resultsTest(t, 40)
	tm.Send(tea.KeyPressMsg{Code: 'j'})
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	// The strip must describe the row the cursor is on, not the first row.
	if !strings.Contains(finalFrame(t, tm), "490 GB × ") {
		t.Error("strip should show the second finding's evidence after one move")
	}
	golden.RequireEqual(t, []byte(finalFrame(t, tm)))
}

func TestResultsScrolls(t *testing.T) {
	tm := resultsTest(t, 40)
	for i := 0; i < 11; i++ {
		tm.Send(tea.KeyPressMsg{Code: 'j'})
	}
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	// The cursor has walked past the bottom of the window, so the list must
	// have scrolled rather than the cursor running off the visible rows.
	frame := finalFrame(t, tm)
	if !strings.Contains(frame, "of 40") {
		t.Error("a list longer than the window should state its scroll position")
	}
	if strings.Contains(frame, "vol-0f066ace215f73000") {
		t.Error("the first finding should have scrolled out of view")
	}
	golden.RequireEqual(t, []byte(finalFrame(t, tm)))
}

func TestResultsPositionHiddenWhenEverythingFits(t *testing.T) {
	tm := resultsTest(t, 3)
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	// "1–3 of 3" is noise. An indicator that is always present stops being read.
	if strings.Contains(finalFrame(t, tm), "of 3") {
		t.Error("scroll position should be hidden when the whole list fits")
	}
}

func TestResultsHasNoDetailScreen(t *testing.T) {
	tm := resultsTest(t, 3)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	tm.Send(tea.KeyPressMsg{Code: 'q'})

	// Enter used to open a detail screen. It now does nothing, so the footer
	// must not advertise it and the breadcrumb must not claim we went there.
	frame := finalFrame(t, tm)
	for _, gone := range []string{"› detail", "↵ detail"} {
		if strings.Contains(frame, gone) {
			t.Errorf("results screen still references %q", gone)
		}
	}
}
