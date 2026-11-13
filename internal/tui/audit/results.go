package audit

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/theme"
)

// resultsBody renders the findings for one check.
//
// Columns are chosen per resource type rather than generically — size and type
// for volumes, and eventually stopped-duration for instances. That is only
// possible because the audit is the unit of navigation: a generic findings list
// would have to flatten every resource to the same columns.
func (m Model) resultsBody() string {
	t := m.theme
	var b strings.Builder

	check, _ := audit.Get(m.activeCheck)
	fs := m.findings

	usd := 0.0
	unpriced := 0
	for _, f := range fs {
		if f.Magnitude.Priced() {
			usd += f.Magnitude.USDPerMonth
		} else {
			unpriced++
		}
	}

	head := fmt.Sprintf("%d finding", len(fs))
	if len(fs) != 1 {
		head += "s"
	}
	// Money only where money is the point. A breached budget carries a count by
	// design, so "4 without a price" would report a limitation that does not
	// exist — nothing was ever going to be priced.
	if check.Magnitude != audit.MagnitudeCount {
		if usd > 0 {
			head += fmt.Sprintf(" · %s/month", money(usd))
		}
		// An unpriced finding must never be silently counted as zero.
		if unpriced > 0 {
			head += fmt.Sprintf(" · %d without a price", unpriced)
		}
	}

	// Scroll position rides on the summary line rather than taking a row of its
	// own — it is only meaningful when the list actually overflows.
	line := "  " + t.TextS().Render(head)
	if pos := m.scrollPosition(); pos != "" {
		gap := m.inner() - 2 - lipgloss.Width(head) - lipgloss.Width(pos)
		if gap > 1 {
			line += strings.Repeat(" ", gap) + t.DimS().Render(pos)
		}
	}
	b.WriteString(line + "\n")
	b.WriteString("  " + t.DimS().Render(check.Name+" · "+string(check.Basis)+" basis") + "\n\n")

	if len(fs) == 0 {
		b.WriteString("  " + t.PositiveS().Render(theme.GlyphOK+" Nothing found.") + "\n")
		return b.String()
	}

	cols := layout(check.Columns, m.inner())
	hdr := ""
	for i, c := range cols {
		if i > 0 {
			hdr += " "
		}
		if c.Right {
			hdr += padLeft(c.Header, c.Width)
			continue
		}
		hdr += padRight(c.Header, c.Width)
	}
	b.WriteString(strings.Repeat(" ", rowIndent) + t.DimS().Render(hdr) + "\n")
	// Same rule as the catalogue, sized off the header itself so the two tables
	// cannot end up with different conventions.
	b.WriteString(strings.Repeat(" ", rowIndent) +
		t.BorderS().Render(strings.Repeat("─", lipgloss.Width(hdr))) + "\n")

	b.WriteString(m.list.View())
	return b.String()
}

// flexPreferred is how much room the flexible column wants; flexFloor is how
// far it will be squeezed to keep every other column on screen. Between the two
// the table sheds columns instead.
const (
	flexPreferred = 20
	flexFloor     = 8
)

// layout sizes a check's declared columns to the available width.
//
// Fixed columns keep their declared width; the flexible one absorbs whatever is
// left, so the table's right edge lands exactly on the frame's. When even the
// flexible column would fall below its floor, columns are shed in the order the
// check declared — the point of the check must survive a narrow terminal, the
// context around it need not.
func layout(cols []audit.Column, inner int) []audit.Column {
	// Capped at the same width as the catalogue, so moving between the two
	// screens does not change how far across the terminal the table reaches.
	if inner > maxGrid {
		inner = maxGrid
	}

	keep := make([]audit.Column, len(cols))
	copy(keep, cols)

	for {
		width, flex := rowIndent+len(keep)-1, -1
		for i, c := range keep {
			if c.Flex {
				flex = i
				continue
			}
			width += c.Width
		}
		if flex < 0 {
			return keep // no flexible column; the declaration is malformed
		}

		if slack := inner - width; slack >= flexPreferred {
			keep[flex].Width = slack
			return keep
		}
		// Too tight. Drop the most sheddable column and try again; if nothing
		// can be shed, the flexible column takes its floor and the frame clips.
		victim := -1
		for i, c := range keep {
			if c.Shed > 0 && (victim < 0 || c.Shed > keep[victim].Shed) {
				victim = i
			}
		}
		if victim < 0 {
			// Nothing left to shed. Squeeze the flexible column instead of
			// letting the table overflow: a truncated resource name is still
			// recognisable, whereas a column pushed past the right edge is
			// simply gone — and on this table that would be OVER, the whole
			// point of the check.
			keep[flex].Width = max(inner-width, flexFloor)
			return keep
		}
		keep = append(keep[:victim:victim], keep[victim+1:]...)
	}
}

// cell renders one column's value for one finding, in the format the check
// declared. Formatting is decided here rather than by the evaluator so the same
// finding can print as a table cell, as JSON, or as prose without the evaluator
// choosing for all three.
func (m Model) cell(f audit.Finding, c audit.Column) string {
	// A handful of keys address the finding's own fields rather than its
	// check-specific Detail. Without these every evaluator would have to copy
	// the region and the savings into Detail just to display them, and the two
	// copies would eventually disagree.
	switch c.Key {
	case "":
		return f.ResourceID
	case "region":
		return f.Region
	case "account":
		return f.Account
	case "owner":
		if f.Owner == "" {
			return "—"
		}
		return f.Owner
	case "savings":
		if !f.Magnitude.Priced() {
			return "—"
		}
		return money(f.Magnitude.USDPerMonth)
	}

	v, ok := f.Detail[c.Key]
	if !ok {
		// Absent is not zero. A budget with no forecast must render blank, not
		// "$0.00", which would read as "forecast to spend nothing".
		return ""
	}

	switch c.Format {
	case audit.FormatMoney:
		if n, ok := toFloat(v); ok {
			return money(n)
		}
	case audit.FormatAmount:
		n, ok := toFloat(v)
		if !ok {
			break
		}
		// Rendered in the finding's own unit: a usage budget measured in GB
		// must not acquire a dollar sign on the way to the screen.
		if unit, _ := f.Detail["unit"].(string); unit != "" && unit != "USD" {
			return fmt.Sprintf("%.2f %s", n, unit)
		}
		return money(n)
	case audit.FormatGB:
		if n, ok := toFloat(v); ok {
			return fmt.Sprintf("%.0f GB", n)
		}
	}
	return fmt.Sprintf("%v", v)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// findingRows renders the scrollable part of the table: one line per finding.
//
// Returned as lines rather than a blob because the viewport measures in lines,
// and a row that ever contains a newline would silently break its arithmetic.
func (m Model) findingRows() []string {
	t := m.theme
	check, _ := audit.Get(m.activeCheck)
	cols := layout(check.Columns, m.inner())

	out := make([]string, 0, len(m.findings))
	for i, f := range m.findings {
		line := ""
		for j, c := range cols {
			if j > 0 {
				line += " "
			}
			v := m.cell(f, c)
			if c.Right {
				line += padLeft(truncate(v, c.Width), c.Width)
				continue
			}
			line += padRight(truncate(v, c.Width), c.Width)
		}

		if i == m.findingCursor {
			out = append(out, strings.Repeat(" ", rowIndent-2)+
				t.SelectedS().Render(theme.GlyphSelected+" "+line))
			continue
		}
		out = append(out, strings.Repeat(" ", rowIndent)+
			t.TextS().Render(strings.TrimRight(line, " ")))
	}
	return out
}

// scrollPosition states which slice of the findings is on screen. Empty when
// everything fits: "1–8 of 8" is noise, and a position indicator that is always
// present stops being read.
func (m Model) scrollPosition() string {
	total := len(m.findings)
	if total == 0 || m.list.TotalLineCount() <= m.list.Height() {
		return ""
	}
	first := m.list.YOffset() + 1
	last := min(total, m.list.YOffset()+m.list.Height())
	return fmt.Sprintf("%d–%d of %d", first, last, total)
}

// findingStrip is the detail pane that follows the cursor: the arithmetic
// behind the number, the facts the columns cannot hold, and the command that
// resolves it.
//
// Showing the working is the honesty mechanism — a dollar figure someone cannot
// check is a dollar figure they should not take into a budget meeting.
func (m Model) findingStrip() string {
	if m.findingCursor >= len(m.findings) {
		return ""
	}
	lines := m.stripFor(m.findings[m.findingCursor])

	// Padded to the height the tallest finding needs, so the pane — and the
	// keybindings under it — hold still while the cursor moves. A footer that
	// shifts as you arrow through a list is disorienting in a way that is hard
	// to name and easy to feel.
	for len(lines) < m.stripHeight() {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// stripFor renders one finding's detail lines, unpadded.
func (m Model) stripFor(f audit.Finding) []string {
	t := m.theme
	lines := make([]string, 0, 3)

	for _, e := range f.Evidence {
		lines = append(lines, t.TextS().Render(e))
	}

	// Facts the table has no column for. Age is what makes "unattached" mean
	// something; owner is what decides whether anyone can act on it.
	facts := []string{}
	if age, ok := f.Detail["age_days"].(int); ok {
		facts = append(facts, fmt.Sprintf("%d days", age))
	}
	facts = append(facts, "account "+f.Account)

	line := t.DimS().Render(strings.Join(facts, " · "))
	switch {
	case f.Owner != "":
		line += t.DimS().Render(" · ") + t.TextS().Render(f.Owner)
	case f.Magnitude.Kind != audit.MagnitudeCount:
		// Only taggable resources can be untagged. A budget is an account-level
		// construct with no owner tag to be missing, so claiming it is untagged
		// would invent a problem.
		line += t.DimS().Render(" · ") + t.WarningS().Render("untagged — no owner")
	}
	lines = append(lines, line)

	return lines
}

// stripHeight is the pane's height: whatever the tallest finding in this result
// set needs. Measured across all of them rather than per finding so the pane
// never changes size as the cursor moves.
func (m Model) stripHeight() int {
	h := 0
	for _, f := range m.findings {
		if n := len(m.stripFor(f)); n > h {
			h = n
		}
	}
	if h < 1 {
		return 1
	}
	return h
}

// runningBody is shown while a scan is in flight. Honest about what it is doing
// rather than freezing the terminal.
func (m Model) runningBody() string {
	t := m.theme
	check, _ := audit.Get(m.activeCheck)

	var b strings.Builder
	b.WriteString("  " + t.AccentS().Render("⠋ ") + t.TextS().Render("Running "+check.Name) + "\n\n")
	b.WriteString("  " + t.DimS().Render("Detecting regions and fetching inventory.") + "\n")
	b.WriteString("  " + t.DimS().Render("No Cost Explorer calls — this is free.") + "\n")
	return b.String()
}

// money formats USD with the precision the amount deserves: cents matter at $0.19
// and are noise at $2,847.
// Money is exported so callers formatting cached values render them the same
// way a live scan does.
func Money(usd float64) string { return money(usd) }

func money(usd float64) string {
	if usd < 100 {
		return fmt.Sprintf("$%.2f", usd)
	}
	whole := fmt.Sprintf("%.0f", usd)
	var out []byte
	for i, c := range []byte(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return "$" + string(out)
}
