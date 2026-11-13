// Package theme is the single source of colour for the whole tool.
//
// Two rules govern every use of it (OVERHAUL.md §A.5):
//
//  1. Never colour alone. Every coloured signal also carries a glyph — ▲▼,
//     ✓✗⚠, ~ for approximate — so meaning survives a monochrome terminal and
//     colour-vision deficiency.
//  2. Apply by meaning, not magnitude. A cost *rise* is Negative while a large
//     *savings* figure is Positive. Same big number, opposite valence.
package theme

import (
	"image/color"

	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

// Theme holds the resolved palette for the terminal's current background.
// Construct it once per program from tea.BackgroundColorMsg and pass it down;
// never call lipgloss.Color directly in view code.
type Theme struct {
	isDark bool

	Text      color.Color // primary content
	Dim       color.Color // labels, secondary, timestamps
	Accent    color.Color // commands, keys, links, bars
	Positive  color.Color // cost down, savings available, ✓
	Negative  color.Color // cost up, errors, breaches, ✗
	Warning   color.Color // stale, approximate, expiring
	Border    color.Color // section rules and frames — structure, recedes
	Rule      color.Color // the title rule, one step brighter so the header reads as a header
	SelBg     color.Color // highlighted row background
	SelFg     color.Color // highlighted row foreground
	TermTitle color.Color // top-bar chrome
}

// New builds the palette for a light or dark terminal background. isDark comes
// from tea.BackgroundColorMsg, which Bubble Tea delivers through Update at
// startup and again whenever the background changes.
func New(isDark bool) Theme {
	pick := lipgloss.LightDark(isDark)
	return Theme{
		isDark: isDark,

		Text:     pick(lipgloss.Color("#1f2328"), lipgloss.Color("#e6edf3")),
		Dim:      pick(lipgloss.Color("#656d76"), lipgloss.Color("#8b949e")),
		Accent:   pick(lipgloss.Color("#0969da"), lipgloss.Color("#79c0ff")),
		Positive: pick(lipgloss.Color("#1a7f37"), lipgloss.Color("#3fb950")),
		Negative: pick(lipgloss.Color("#cf222e"), lipgloss.Color("#f85149")),
		Warning:  pick(lipgloss.Color("#9a6700"), lipgloss.Color("#d29922")),
		Border:   pick(lipgloss.Color("#d0d7de"), lipgloss.Color("#30363d")),
		// One step up from Border. The top rule carries the tool name and
		// breadcrumb, so it is a header rather than a divider and should not
		// recede as far as the section rules do.
		Rule: pick(lipgloss.Color("#afb8c1"), lipgloss.Color("#484f58")),

		// Selection is a warm tint, not saturated orange: it marks where the
		// cursor is, which is the least semantic thing on screen. A background
		// reads as position; saturated colour would read as meaning and
		// collide with Warning.
		SelBg: pick(lipgloss.Color("#fff1e5"), lipgloss.Color("#3d2a12")),
		SelFg: pick(lipgloss.Color("#bc4c00"), lipgloss.Color("#ffa657")),

		TermTitle: pick(lipgloss.Color("#656d76"), lipgloss.Color("#8b949e")),
	}
}

// IsDark reports the background the palette was built for.
func (t Theme) IsDark() bool { return t.isDark }

// Style helpers. View code composes these rather than building styles inline,
// so a palette change is a one-file change.

func (t Theme) S() lipgloss.Style         { return lipgloss.NewStyle() }
func (t Theme) TextS() lipgloss.Style     { return lipgloss.NewStyle().Foreground(t.Text) }
func (t Theme) DimS() lipgloss.Style      { return lipgloss.NewStyle().Foreground(t.Dim) }
func (t Theme) AccentS() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Accent) }
func (t Theme) PositiveS() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Positive) }
func (t Theme) NegativeS() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Negative) }
func (t Theme) WarningS() lipgloss.Style  { return lipgloss.NewStyle().Foreground(t.Warning) }
func (t Theme) BorderS() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Border) }
func (t Theme) RuleS() lipgloss.Style     { return lipgloss.NewStyle().Foreground(t.Rule) }
func (t Theme) SelectedS() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.SelBg).Foreground(t.SelFg)
}

// HelpStyles themes the bubbles help component. Without this the footer renders
// in the library's own greys, which is the one part of the UI that would not
// follow the palette — and the keys would not read as interactive.
func (t Theme) HelpStyles() help.Styles {
	return help.Styles{
		ShortKey:       t.AccentS(),
		ShortDesc:      t.DimS(),
		ShortSeparator: t.BorderS(),
		FullKey:        t.AccentS(),
		FullDesc:       t.DimS(),
		FullSeparator:  t.BorderS(),
		Ellipsis:       t.DimS(),
	}
}

// Glyphs. Held here so the "never colour alone" rule has one place to look,
// and so a terminal with a poor font can be accommodated centrally later.
const (
	GlyphOK          = "✓"
	GlyphFail        = "✗"
	GlyphWarn        = "⚠"
	GlyphUp          = "▲"
	GlyphDown        = "▼"
	GlyphSelected    = "▸"
	GlyphExpanded    = "▾"
	GlyphCollapsed   = "▸"
	GlyphApproximate = "~"
	GlyphArrow       = "→"
)
