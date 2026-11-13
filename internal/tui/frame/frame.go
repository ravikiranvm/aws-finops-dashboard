// Package frame draws the chrome every interactive screen shares: an outer
// border carrying the tool name, breadcrumb and identity in its top rule, and a
// bottom section holding the keybindings.
//
// The frame fills the terminal. In the alternate screen the application owns
// the whole display, so anything smaller leaves dead space around a floating
// box (OVERHAUL.md D-A3).
package frame

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/theme"
)

// Box-drawing set. Kept in one place so a --no-border mode, or a fallback for
// fonts with poor box coverage, is a single substitution rather than a hunt.
const (
	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxTeeLeft     = "├"
	boxTeeRight    = "┤"
)

// Frame is the chrome around one screen's content.
type Frame struct {
	Theme  theme.Theme
	Width  int
	Height int

	// Tool is the left of the top rule, e.g. "finops 3.0".
	Tool string
	// Breadcrumb follows it: "audit", "audit › gp2 → gp3 › detail".
	Breadcrumb string
	// Identity is the right of the top rule, already abbreviated by the caller.
	Identity string

	// Keys is the pre-rendered help line, generated from key bindings rather
	// than hand-written (OVERHAUL.md D-A4).
	Keys string
	// Command is the "→ finops …" line. Empty on screens with no view state
	// worth reproducing.
	Command string
	// Detail is an optional contextual section above the keys, describing
	// whatever row is selected.
	Detail string

	// Center vertically centres content. Right for sparse, single-decision
	// screens; wrong for lists, where content grows downward and a centred
	// block would shift on every keystroke.
	Center bool

	// Style selects how much box drawing to draw. See BorderStyle.
	Style BorderStyle

	// Pad is the horizontal breathing room between the frame edge and content,
	// in columns. Zero means DefaultPad.
	Pad int
}

// DefaultPad is the standard left/right margin. One column reads as content
// jammed against the edge; three starts wasting width on an 80-column terminal.
const DefaultPad = 3

// BorderStyle is how much of the frame is drawn.
type BorderStyle int

const (
	// BorderRules draws only the horizontal rules: a titled top rule, section
	// dividers, and a bottom rule. The verticals are omitted because they sit
	// at the far edges doing no structural work, and they cost two columns
	// that the impact column would rather have.
	BorderRules BorderStyle = iota
	// BorderFull draws a complete box.
	BorderFull
	// BorderNone drops the bottom rule too, for terminals with poor
	// box-drawing coverage.
	BorderNone
)

func (f Frame) boxed() bool { return f.Style == BorderFull }

// maxCenterPad caps how far content is pushed down when centring. On a tall
// terminal true centring strands a short menu mid-void; upper-middle reads
// better and keeps the eye near the top.
const maxCenterPad = 6

// Inner is the usable width inside the frame: terminal width less the two
// border columns and one space of padding either side.
func (f Frame) pad() int {
	if f.Pad <= 0 {
		return DefaultPad
	}
	return f.Pad
}

func (f Frame) Inner() int {
	w := f.Width - 2*f.pad()
	if f.boxed() {
		w -= 2 // plus the vertical rules
	}
	if w < 20 {
		return 20
	}
	return w
}

func (f Frame) chromeHeight() int {
	n := 1 // top rule (carries the title, so it costs nothing extra)
	n++    // blank line under it
	n++    // rule above the keys
	n++    // keys
	if f.Style != BorderNone {
		n++ // bottom rule
	}
	if f.Command != "" {
		n++
	}
	if f.Detail != "" {
		// Render emits a blank spacer, a rule, and then the text — three costs,
		// not two. Undercounting here pushed the whole frame one row past the
		// terminal, which in the alternate screen scrolls the top rule away.
		lines := strings.Count(f.Detail, "\n") + 1
		n += 2 + lines
	}
	return n
}

// ContentHeight is the number of lines available to the screen's own content.
func (f Frame) ContentHeight() int {
	h := f.Height - f.chromeHeight()
	if h < 1 {
		return 1
	}
	return h
}

// Render wraps content in the frame. Content is padded to exactly
// ContentHeight lines so the bottom bar never moves as content changes — a
// footer that jumps while you navigate is disorienting.
func (f Frame) Render(content string) string {
	var b strings.Builder

	b.WriteString(f.topRule())
	b.WriteString("\n")
	b.WriteString(f.row(""))
	b.WriteString("\n")

	for _, line := range strings.Split(f.fitContent(content), "\n") {
		b.WriteString(f.row(line))
		b.WriteString("\n")
	}

	if f.Detail != "" {
		b.WriteString(f.row(""))
		b.WriteString("\n")
		b.WriteString(f.teeRule())
		b.WriteString("\n")
		// Rendered as-is: callers style their own detail, because a screen that
		// needs a warning colour or an accented command inside the strip cannot
		// undo a blanket dim applied here.
		for _, line := range strings.Split(f.Detail, "\n") {
			b.WriteString(f.row(line))
			b.WriteString("\n")
		}
	}

	b.WriteString(f.teeRule())
	b.WriteString("\n")
	b.WriteString(f.row(f.Keys))
	if f.Command != "" {
		b.WriteString("\n")
		b.WriteString(f.row(f.Theme.AccentS().Render(f.Command)))
	}
	if rule := f.bottomRule(); rule != "" {
		b.WriteString("\n")
		b.WriteString(rule)
	}

	return b.String()
}

// row draws one content line between the vertical borders, padding to the
// exact inner width. Padding is by display width, not rune count: a single
// glyph measured wrong leaves a ragged right edge down the whole screen.
func (f Frame) row(content string) string {
	bx := f.Theme.BorderS()
	// Overflow would push the right border off the grid and leave a ragged
	// edge down the whole screen, so clip rather than let it happen. Callers
	// size their own columns; this is the last line of defence.
	if lipgloss.Width(content) > f.Inner() {
		content = clip(content, f.Inner())
	}
	pad := f.Inner() - lipgloss.Width(content)
	if pad < 0 {
		pad = 0
	}
	margin := strings.Repeat(" ", f.pad())
	if !f.boxed() {
		// No verticals: trailing padding would only add invisible whitespace.
		return margin + content
	}
	return bx.Render(boxVertical) + margin + content +
		strings.Repeat(" ", pad) + margin + bx.Render(boxVertical)
}

// topRule carries the title, which is why the border costs about one net row
// rather than two: the header line becomes the top edge.
//
//	┌─ finops 3.0 · audit ────────────────── prod-mgmt · org 14 ─┐
func (f Frame) topRule() string {
	t := f.Theme
	bx := t.RuleS()

	left := f.Tool
	if f.Breadcrumb != "" {
		left += " · " + f.Breadcrumb
	}

	// The title lives in the rule either way; only the corners differ.
	lead, tail := boxTopLeft+boxHorizontal, boxHorizontal+boxTopRight
	if !f.boxed() {
		lead, tail = boxHorizontal+boxHorizontal, boxHorizontal+boxHorizontal
	}

	// ─ left ─…─ right ─  — the dashes absorb whatever is left over.
	fixed := 2 + 1 + lipgloss.Width(left) + 1
	if f.Identity != "" {
		fixed += 1 + lipgloss.Width(f.Identity) + 2 // " " + identity + " ─"
	}
	dashes := f.Width - fixed - 1 // -1 for the closing corner
	if dashes < 1 {
		// No room for the identity: the breadcrumb tells you where you are,
		// which matters more than repeating who you are on every screen.
		dashes = f.Width - (2 + 1 + lipgloss.Width(left) + 1) - 1
		if dashes < 1 {
			return bx.Render(strings.Repeat(boxHorizontal, f.Width))
		}
		return bx.Render(lead+" ") + t.TextS().Render(left) +
			bx.Render(" "+strings.Repeat(boxHorizontal, dashes)+tail)
	}

	out := bx.Render(lead+" ") + t.TextS().Render(left) + bx.Render(" ")
	out += bx.Render(strings.Repeat(boxHorizontal, dashes))
	if f.Identity != "" {
		out += bx.Render(" ") + t.DimS().Render(f.Identity) + bx.Render(" ")
	}
	return out + bx.Render(tail)
}

func (f Frame) teeRule() string {
	bx := f.Theme.BorderS()
	if !f.boxed() {
		return bx.Render(strings.Repeat(boxHorizontal, f.Width))
	}
	return bx.Render(boxTeeLeft + strings.Repeat(boxHorizontal, f.Width-2) + boxTeeRight)
}

func (f Frame) bottomRule() string {
	bx := f.Theme.BorderS()
	switch f.Style {
	case BorderNone:
		return ""
	case BorderFull:
		return bx.Render(boxBottomLeft + strings.Repeat(boxHorizontal, f.Width-2) + boxBottomRight)
	default:
		return bx.Render(strings.Repeat(boxHorizontal, f.Width))
	}
}

// fitContent pads content to exactly ContentHeight lines, centring the padding
// when Center is set and otherwise leaving it all at the bottom.
func (f Frame) fitContent(content string) string {
	want := f.ContentHeight()
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	if len(lines) >= want {
		return strings.Join(lines[:want], "\n")
	}

	pad := want - len(lines)
	above, below := 0, pad
	if f.Center {
		above = min(pad/2, maxCenterPad)
		below = pad - above
	}
	return strings.Repeat("\n", above) + strings.Join(lines, "\n") + strings.Repeat("\n", below)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// clip truncates to a display width, preserving any leading ANSI styling.
func clip(s string, width int) string {
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > width {
		r = r[:len(r)-1]
	}
	return string(r)
}
