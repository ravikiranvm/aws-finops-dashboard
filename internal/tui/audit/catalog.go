// Package audit renders the audit screens.
package audit

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/frame"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/theme"
)

// Result is the outcome of the last run of one check. A nil Result means the
// check has never been run, which is a state the catalogue shows explicitly
// rather than hiding — coverage gaps matter as much as findings.
type Result struct {
	Findings int
	// Impact is pre-formatted because categories are not commensurable:
	// savings, unattributed spend, and counts cannot share a column format.
	Impact string
	// Age is a human span since the run, e.g. "26m ago".
	Age string
	// Stale marks a result old enough not to trust.
	Stale bool
	// Value is the numeric magnitude, used only for ordering. Impact is the
	// string shown; this is what sorts it, because "$1,204/mo" and "$890/mo"
	// do not compare as text.
	Value float64
}

// row is one line in the list: a category header, a check, or a spacer.
//
// Spacers are part of the row list rather than injected at render time so the
// cursor can skip them — a blank line you can land on feels broken.
type row struct {
	header   bool
	spacer   bool
	category audit.Category
	check    audit.Check
}

type keyMap struct {
	Up       key.Binding
	Back     key.Binding
	Down     key.Binding
	Expand   key.Binding
	Results  key.Binding
	Run      key.Binding
	RunAll   key.Binding
	Settings key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Expand, k.Results, k.Run, k.RunAll, k.Settings, k.Back, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Expand},
		{k.Results, k.Run, k.RunAll},
		{k.Settings, k.Back, k.Help, k.Quit},
	}
}

func defaultKeys() keyMap {
	return keyMap{
		Up: key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "move")),
		// Back and Quit are separate bindings on purpose. Folding them together
		// meant ctrl+c navigated instead of quitting, and ctrl+c must always
		// quit — no screen gets to trap it.
		Back: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "down")),
		// "space", not " ": v2 matches on the key's string name, so a literal
		// space character never matches. See docs/STACK.md.
		Expand:   key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "expand")),
		Results:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "results")),
		Run:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")),
		RunAll:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "run all")),
		Settings: key.NewBinding(key.WithKeys(","), key.WithHelp(",", "settings")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// screen is which view the model is currently showing. Held as state on one
// model rather than as separate models, so navigation is a field change and the
// findings do not have to be threaded between components.
type screen int

const (
	screenCatalog screen = iota
	screenRunning
	screenResults
)

// RunFunc executes one check and returns its findings. Injected so the TUI has
// no dependency on AWS: tests drive it with a stub, and the real one is wired in
// by cmd/finops.
type RunFunc func(checkID string) ([]audit.Finding, error)

// PersistFunc records a completed scan so it survives quitting. Injected for
// the same reason as RunFunc: the TUI decides what happened, it does not own
// where that goes.
type PersistFunc func(checkID string, findings []audit.Finding)

// scanDone carries a completed scan back into Update.
type scanDone struct {
	checkID  string
	findings []audit.Finding
	err      error
}

// summarise turns a finished scan into the catalogue row for that check.
//
// A run that found nothing still produces a Result — "0 findings, just now" is
// meaningfully different from "never run", and collapsing them would hide the
// fact that we looked.
func summarise(checkID string, findings []audit.Finding) Result {
	r := Result{Findings: len(findings), Age: "just now"}

	check, ok := audit.Get(checkID)
	if !ok {
		return r
	}

	switch check.Magnitude {
	case audit.MagnitudeSavings, audit.MagnitudeUnallocated:
		var total float64
		unpriced := 0
		for _, f := range findings {
			if f.Magnitude.Priced() {
				total += f.Magnitude.USDPerMonth
			} else {
				unpriced++
			}
		}
		r.Value = total
		switch {
		case len(findings) == 0:
			r.Impact = "none"
		case total == 0 && unpriced > 0:
			// Findings exist but none could be priced. Showing "$0" would
			// claim we checked and found nothing worth saving.
			r.Impact = "unpriced"
		default:
			r.Impact = money(total) + "/mo"
		}
	default:
		r.Value = float64(len(findings))
		if len(findings) == 0 {
			r.Impact = "none"
			break
		}
		// "3 breached", not a bare "3": in a column of dollar figures a lone
		// number reads as an amount, and FOUND already carries the count.
		r.Impact = fmt.Sprintf("%d %s", len(findings), check.CountLabel)
	}
	return r
}

// Model is the audit catalogue: every check the tool knows how to run, grouped
// by the question it answers, with the state of its last run.
type Model struct {
	screen  screen
	run     RunFunc
	persist PersistFunc

	activeCheck   string
	findings      []audit.Finding
	findingCursor int
	runErr        error
	// list scrolls the findings table. Only the rows scroll: the summary, the
	// column header and the detail strip are chrome and stay put.
	list viewport.Model

	checks    map[audit.Category][]audit.Check
	results   map[string]Result // check ID → last run; absent means never run
	collapsed map[audit.Category]bool

	rows   []row
	cursor int

	keys    keyMap
	help    help.Model
	theme   theme.Theme
	width   int
	height  int
	version string
	// identity is the abbreviated right slot of the top bar.
	identity string
}

// NewCatalog builds the catalogue from the check registry. results may be nil,
// which is the honest first-run state.
// SetPersist attaches the callback used to remember completed scans.
func (m Model) SetPersist(p PersistFunc) Model { m.persist = p; return m }

func NewCatalog(results map[string]Result, version, identity string, run RunFunc) Model {
	m := Model{
		run:       run,
		checks:    audit.ByCategory(),
		results:   results,
		collapsed: map[audit.Category]bool{},
		keys:      defaultKeys(),
		help:      help.New(),
		list:      viewport.New(),
		theme:     theme.New(true),
		width:     80,
		height:    24,
		version:   version,
		identity:  identity,
	}
	if m.results == nil {
		m.results = map[string]Result{}
	}

	// Only advertise what works. Run-all and settings are still unbuilt, so
	// their bindings stay off and vanish from the footer; a disabled binding
	// cannot promise more than the tool delivers.
	m.keys.RunAll.SetEnabled(false)
	m.keys.Settings.SetEnabled(false)
	if run == nil {
		m.keys.Run.SetEnabled(false)
		m.keys.Results.SetEnabled(false)
	}

	m.rebuild()
	return m
}

// rebuild flattens categories and checks into the visible row list.
func (m *Model) rebuild() {
	m.rows = nil
	for _, cat := range audit.Order {
		checks := m.checks[cat]
		if len(checks) == 0 {
			continue
		}
		// A blank row before each group but the first. Categories are the
		// screen's structure; without separation the list reads as one long
		// run of rows with occasional bold text in it.
		if len(m.rows) > 0 {
			m.rows = append(m.rows, row{spacer: true})
		}
		m.rows = append(m.rows, row{header: true, category: cat})
		if m.collapsed[cat] {
			continue
		}
		// Order by magnitude so the money is at the top. Never-run checks sort
		// last: they have no claim on attention until someone runs them.
		ordered := append([]audit.Check(nil), checks...)
		sort.SliceStable(ordered, func(i, j int) bool {
			return m.results[ordered[i].ID].Value > m.results[ordered[j].ID].Value
		})
		for _, c := range ordered {
			m.rows = append(m.rows, row{category: cat, check: c})
		}
	}
	if m.cursor >= len(m.rows) {
		m.cursor = max(0, len(m.rows)-1)
	}
	for m.cursor > 0 && m.cursor < len(m.rows) && m.rows[m.cursor].spacer {
		m.cursor--
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.theme = theme.New(msg.IsDark())
		m.help.Styles = m.theme.HelpStyles()
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.syncList()
		return m, nil

	case scanDone:
		m.findings, m.runErr = msg.findings, msg.err
		m.findingCursor = 0
		// Record the run so the catalogue reflects it. Without this the tool
		// says "never" immediately after a successful scan, and the sort order
		// never learns which checks actually found money.
		if msg.err == nil {
			m.results[msg.checkID] = summarise(msg.checkID, msg.findings)
			m.rebuild()
			if m.persist != nil {
				m.persist(msg.checkID, msg.findings)
			}
		}
		m.screen = screenResults
		m.syncList()
		return m, nil

	case tea.KeyPressMsg:
		switch m.screen {
		case screenRunning:
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			if key.Matches(msg, m.keys.Back) {
				m.screen = screenCatalog
			}
			return m, nil

		case screenResults:
			switch {
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keys.Back):
				m.screen = screenCatalog
			case key.Matches(msg, m.keys.Up):
				if m.findingCursor > 0 {
					m.findingCursor--
					m.syncList()
				}
			case key.Matches(msg, m.keys.Down):
				if m.findingCursor < len(m.findings)-1 {
					m.findingCursor++
					m.syncList()
				}
			case key.Matches(msg, m.keys.Run):
				return m.startRun(m.activeCheck)
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Up):
			m.move(-1)
		case key.Matches(msg, m.keys.Down):
			m.move(+1)
		case key.Matches(msg, m.keys.Expand):
			if r := m.current(); r.header {
				m.collapsed[r.category] = !m.collapsed[r.category]
				m.rebuild()
			}
		case key.Matches(msg, m.keys.Run):
			if r := m.current(); !r.header && m.run != nil {
				return m.startRun(r.check.ID)
			}
		}
	}
	return m, nil
}

// startRun kicks off a scan as a tea.Cmd, so the UI keeps rendering while AWS
// calls are in flight rather than freezing.
func (m Model) startRun(checkID string) (tea.Model, tea.Cmd) {
	m.activeCheck = checkID
	m.screen = screenRunning
	run := m.run
	return m, func() tea.Msg {
		findings, err := run(checkID)
		return scanDone{checkID: checkID, findings: findings, err: err}
	}
}

// syncList refills the findings viewport and keeps the cursor on screen.
//
// Called on every event that can change what the list holds or how much of it
// fits: a completed scan, a resize, and each cursor move.
func (m *Model) syncList() {
	m.list.SetWidth(m.inner())
	m.list.SetHeight(m.listHeight())
	m.list.SetContentLines(m.findingRows())
	m.list.EnsureVisible(m.findingCursor, 0, 0)
}

// listHeight is what the frame leaves for the scrolling rows once the summary,
// the column header, the rule and the detail strip have taken their share.
func (m Model) listHeight() int {
	f := frame.Frame{
		Theme: m.theme, Width: m.width, Height: m.height,
		Style: frame.BorderRules, Keys: " ", Detail: m.blankStrip(),
	}
	// 2 summary lines, 1 blank, the column header, and its rule.
	h := f.ContentHeight() - 5
	if h < 1 {
		return 1
	}
	return h
}

// blankStrip is a detail pane of the right height but no content, used to ask
// the frame how much room the real one will take. Measured this way rather than
// from the selected finding so the list height cannot change as the cursor
// moves down.
func (m Model) blankStrip() string {
	return strings.Repeat("\n", m.stripHeight()-1)
}

// move steps the cursor, skipping spacers so navigation never rests on a
// blank line.
func (m *Model) move(delta int) {
	next := m.cursor
	for {
		next += delta
		if next < 0 || next >= len(m.rows) {
			return // would run off the end; stay put
		}
		if !m.rows[next].spacer {
			m.cursor = next
			return
		}
	}
}

func (m Model) current() row {
	if m.cursor < len(m.rows) {
		return m.rows[m.cursor]
	}
	return row{}
}

func (m Model) View() tea.View {
	f := frame.Frame{
		Theme:    m.theme,
		Width:    m.width,
		Height:   m.height,
		Tool:     "finops " + m.version,
		Identity: m.identity,
		Keys:     m.help.View(m.keysFor(m.screen)),
		// A list: content grows downward, so top-aligned. Centring would make
		// the whole block shift every time a category expands.
		Style:  frame.BorderRules,
		Center: false,
	}

	var body string
	switch m.screen {
	case screenRunning:
		f.Breadcrumb = "audit › running"
		body = m.runningBody()
	case screenResults:
		f.Breadcrumb = "audit › " + m.checkName(m.activeCheck)
		f.Detail = m.findingStrip()
		body = m.resultsBody()
	default:
		f.Breadcrumb = "audit"
		f.Detail = m.detail()
		body = m.body()
	}

	v := tea.NewView(f.Render(body))
	v.AltScreen = true
	return v
}

// keysFor returns the bindings that actually do something on a given screen.
// Disabled bindings vanish from the generated help, so the footer can never
// advertise "space expand" on a screen with nothing to expand.
func (m Model) keysFor(sc screen) keyMap {
	k := m.keys
	switch sc {
	case screenCatalog:
		// Nothing above the catalogue, so "back" would be a lie.
		k.Back.SetEnabled(false)
	case screenRunning:
		k.Up.SetEnabled(false)
		k.Down.SetEnabled(false)
		k.Expand.SetEnabled(false)
		k.Results.SetEnabled(false)
		k.Run.SetEnabled(false)
		k.Help.SetEnabled(false)
	case screenResults:
		k.Expand.SetEnabled(false)
		// Nothing to drill into any more: the detail follows the cursor, so
		// "↵ detail" would promise a screen that no longer exists.
		k.Results.SetEnabled(false)
		k.Run.SetHelp("r", "re-run")
	}
	return k
}

func (m Model) checkName(id string) string {
	if c, ok := audit.Get(id); ok {
		return c.Name
	}
	return id
}

// detail describes the selected row in the strip above the keys. This is the
// catalogue's answer to "what does this check actually do", kept out of the
// list so 7 checks do not become 21 lines.
func (m Model) detail() string {
	r := m.current()
	if r.header {
		return m.theme.DimS().Render(
			fmt.Sprintf("%s · %d checks", r.category.Title(), len(m.checks[r.category])))
	}
	if r.check.ID == "" {
		return ""
	}
	head := fmt.Sprintf("%s · %s", r.check.Name, r.check.Category.Title())
	if r.check.Basis != audit.BasisNone {
		head += fmt.Sprintf(" · %s basis", r.check.Basis)
	}
	// Styled here rather than by the frame: the frame renders the strip as-is
	// so screens that need colour inside it are not fighting a blanket dim.
	return m.theme.TextS().Render(head) + "\n" +
		m.theme.DimS().Render(wrap(r.check.Description, m.width-4))
}

func (m Model) body() string {
	t := m.theme
	var b strings.Builder

	b.WriteString(m.summary() + "\n\n")

	// Column headers. Three unlabelled numeric columns is a guessing game —
	// "8" could be findings, dollars, or days without them. Indented to line up
	// with the check rows, which sit two spaces deeper than category headers.
	nameW, foundW, ageW, impactW := catalogCols(m.inner())
	head := padRight("AUDIT", nameW) + " " + padLeft("FOUND", foundW)
	if ageW > 0 {
		head += "  " + padRight("LAST RUN", ageW)
	}
	head += " " + padLeft("IMPACT", impactW)
	b.WriteString(strings.Repeat(" ", rowIndent) + t.DimS().Render(head) + "\n")
	// A rule under the labels, spanning exactly the column grid rather than the
	// full width — it marks where the table starts, so it should end where the
	// table ends.
	b.WriteString(strings.Repeat(" ", rowIndent) +
		t.BorderS().Render(strings.Repeat("─", m.gridSpan()-rowIndent)) + "\n")

	for i, r := range m.rows {
		selected := i == m.cursor
		if r.spacer {
			b.WriteString("\n")
			continue
		}
		if r.header {
			b.WriteString(m.renderHeader(r, selected))
		} else {
			b.WriteString(m.renderCheck(r, selected))
		}
	}

	_ = t
	return b.String()
}

// summary is the headline. With nothing run it states coverage rather than
// inventing a total, because a total over an empty set would imply we looked.
func (m Model) summary() string {
	t := m.theme
	total := 0
	for _, cs := range m.checks {
		total += len(cs)
	}
	if len(m.results) == 0 {
		return t.DimS().Render(fmt.Sprintf("No audits run yet · %d available", total))
	}
	return t.DimS().Render(fmt.Sprintf("%d of %d audits run", len(m.results), total))
}

func (m Model) renderHeader(r row, selected bool) string {
	t := m.theme
	glyph := theme.GlyphExpanded
	if m.collapsed[r.category] {
		glyph = theme.GlyphCollapsed
	}

	label := strings.ToUpper(r.category.Title())
	line := glyph + " " + label

	// The count is only worth the space while the group is shut. Expanded, the
	// rows underneath already say how many there are, and repeating it puts a
	// number in the impact column's lane that is not an impact.
	if m.collapsed[r.category] {
		count := fmt.Sprintf("%d audits", len(m.checks[r.category]))
		if len(m.checks[r.category]) == 1 {
			count = "1 audit"
		}
		// The header spans the same grid as the rows beneath it, so its right
		// edge lines up with the impact column, not with the terminal edge.
		pad := max(1, m.gridSpan()-lipgloss.Width(glyph)-1-lipgloss.Width(label)-lipgloss.Width(count))
		line += strings.Repeat(" ", pad) + count
	}

	if selected {
		return t.SelectedS().Render(line) + "\n"
	}
	return t.DimS().Render(line) + "\n"
}

func (m Model) renderCheck(r row, selected bool) string {
	t := m.theme

	age, impact, findings := "never", "—", ""
	res, ran := m.results[r.check.ID]
	if ran {
		age, impact, findings = res.Age, res.Impact, fmt.Sprintf("%d", res.Findings)
	}

	// padRight/padLeft rather than %-11s: Printf pads by rune count, but glyphs
	// like ⚠ occupy two terminal cells, so a row containing one drifts out of
	// alignment with every other row.
	nameW, foundW, ageW, impactW := catalogCols(m.inner())
	// The impact column is styled separately from the rest of the row. It is
	// the reason this screen exists, so rendering it at the same weight as a
	// timestamp buries the only number anyone came to read.
	impactCell := padLeft(impact, impactW)
	if ran {
		switch r.check.Magnitude {
		case audit.MagnitudeSavings:
			// Money you could stop spending — the good kind of large number.
			impactCell = t.PositiveS().Render(impactCell)
		case audit.MagnitudeUnallocated:
			// Spend you cannot attribute. Not savings, not an error: a gap.
			impactCell = t.WarningS().Render(impactCell)
		default:
			impactCell = t.TextS().Render(impactCell)
		}
	} else {
		impactCell = t.DimS().Render(impactCell)
	}

	// Metadata recedes so the eye lands on the name and the money.
	body := t.TextS().Render(padRight(truncate(r.check.Name, nameW), nameW)) +
		" " + t.DimS().Render(padLeft(findings, foundW))
	if ageW > 0 {
		body += "  " + t.DimS().Render(padRight(age, ageW))
	}
	line := body + " " + impactCell

	// The stale marker goes AFTER the last column, never inside one. ⚠ is an
	// East-Asian Ambiguous character: lipgloss measures it as one cell, but
	// many terminals render it as two, so a row containing it would drift out
	// of alignment on some machines and not others. Trailing content cannot
	// misalign anything that follows it, because nothing does.
	if ran && res.Stale {
		line += " " + t.WarningS().Render(theme.GlyphWarn)
	}
	if selected {
		// The selection background wins over per-cell colour: a row half in
		// the highlight and half in green reads as two things, not one.
		plain := padRight(truncate(r.check.Name, nameW), nameW) + " " + padLeft(findings, foundW)
		if ageW > 0 {
			plain += "  " + padRight(age, ageW)
		}
		plain += " " + padLeft(impact, impactW)
		if ran && res.Stale {
			plain += " " + theme.GlyphWarn
		}
		return strings.Repeat(" ", rowIndent-2) +
			t.SelectedS().Render(theme.GlyphSelected+" "+plain) + "\n"
	}
	return strings.Repeat(" ", rowIndent) + line + "\n"
}

// wrap breaks text to width on word boundaries, indenting continuation lines
// so the detail strip reads as one paragraph.
func wrap(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n  ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// gridSpan is the total width of the column grid, from the left edge of the
// screen to the right edge of the impact column. Derived from catalogCols so
// the header rule, the category rows and the data rows cannot drift apart.
func (m Model) gridSpan() int {
	nameW, foundW, ageW, impactW := catalogCols(m.inner())
	span := rowIndent + nameW + 1 + foundW + 1 + impactW
	if ageW > 0 {
		span += ageW + 2
	}
	return span
}

// maxGrid caps the table's total width. The grid fills the terminal up to this
// point and no further: past roughly this width the eye has to travel too far
// from an audit's name to its dollar figure to connect them, and the surplus is
// better spent as margin than as a gulf inside every row.
const maxGrid = 140

// minName is the narrowest an audit name may be squeezed. Below this the names
// stop being distinguishable from each other, so the frame clips instead.
const minName = 24

// catalogCols computes column widths for the available inner width.
//
// Numeric columns are fixed — they never need more than their content — and
// the name column absorbs all the remaining width, so the grid's right edge
// lands exactly on the frame's inner edge rather than short of it.
func catalogCols(inner int) (name, found, age, impact int) {
	found, age, impact = 5, 11, 12
	if inner > maxGrid {
		inner = maxGrid
	}

	// Shed the least important column rather than clipping the most important
	// one. Narrow terminals used to truncate IMPACT to "IM" — the money is the
	// reason the screen exists, so LAST RUN goes first.
	if inner < rowIndent+minName+found+age+impact+4 {
		age = 0
	}

	// gaps must match the separators gridSpan sums: one space before FOUND, one
	// before IMPACT, and two before LAST RUN when it is shown. Getting this
	// wrong leaves the table ending short of the frame, which reads as a
	// rendering bug rather than a layout choice.
	gaps := 2
	if age > 0 {
		gaps += 2
	}
	name = inner - rowIndent - found - impact - age - gaps
	if name < minName {
		name = minName
	}
	return name, found, age, impact
}

// rowIndent is where every data row and column header starts. Category headers
// sit two columns to its left so the hierarchy is visible.
const rowIndent = 2

// padRight and padLeft pad to a target *display* width, counting terminal cells
// rather than runes. Go's %-Ns verb counts runes, so a single ⚠ or ▲ silently
// shifts a column by one cell and the table stops lining up.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func padLeft(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// inner is the usable width inside the frame, mirroring frame.Frame.Inner.
func (m Model) inner() int {
	w := m.width - 2*frame.DefaultPad
	if w < 20 {
		return 20
	}
	return w
}

// truncate shortens to a display width, marking the cut so a clipped name is
// never mistaken for the whole name.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 1 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
