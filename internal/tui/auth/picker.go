// Package auth renders the interactive credential screens.
package auth

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/frame"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/tui/theme"
)

// Choice is what the user picked. The caller acts on it after the program
// exits — the TUI decides, it does not execute.
type Choice struct {
	Kind core.Kind
	// Name is the profile or SSO session name, where the kind needs one.
	Name string
	// Cancelled is true when the user quit without choosing.
	Cancelled bool
}

// option is one selectable line in the picker.
type option struct {
	label  string
	hint   string
	kind   core.Kind
	name   string
	detail string
}

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Help   key.Binding
	Quit   key.Binding
}

// ShortHelp and FullHelp are what the help component renders. Declaring the
// binding once and deriving both means the visible keys cannot drift from the
// keys that actually work.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Select, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down}, {k.Select}, {k.Help, k.Quit}}
}

func defaultKeys() keyMap {
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑↓", "move")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓", "down")),
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "select")),
		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:   key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// Model is the credential picker.
type Model struct {
	discovery core.Discovery
	options   []option
	cursor    int

	// keys is per-model rather than package-level so bindings can be disabled
	// for this instance — a disabled binding disappears from the help view
	// automatically, so the footer cannot advertise a key that does nothing.
	keys keyMap

	help    help.Model
	theme   theme.Theme
	width   int
	height  int
	version string

	choice Choice
	done   bool
}

// NewPicker builds the picker from what discovery found. It only offers
// sources that are actually usable, so the list never contains a dead end.
func NewPicker(d core.Discovery, version string) Model {
	m := Model{
		discovery: d,
		keys:      defaultKeys(),
		help:      help.New(),
		theme:     theme.New(true), // replaced by the real background on start
		width:     80,
		height:    24,
		version:   version,
	}

	for _, s := range d.Sources {
		if !s.Available {
			continue
		}
		switch s.Kind {
		case core.KindSSO:
			m.options = append(m.options, option{
				label: fmt.Sprintf("Sign in to %q", s.Name),
				hint:  "runs aws sso login",
				kind:  core.KindSSO, name: s.Name,
			})
		case core.KindEnv:
			m.options = append(m.options, option{
				label: "Use environment credentials", kind: core.KindEnv,
			})
		case core.KindOIDC:
			m.options = append(m.options, option{
				label: "Use web identity (OIDC)", kind: core.KindOIDC,
			})
		case core.KindAmbient:
			m.options = append(m.options, option{
				label: "Use " + s.Detail, kind: core.KindAmbient,
			})
		}
	}
	// Profiles are enumerated individually: "4 found" is a fact, not a choice.
	for _, p := range d.Profiles {
		m.options = append(m.options, option{
			label: "Use profile  " + p, kind: core.KindProfile, name: p,
		})
	}

	// With no usable source there is nothing to move through or select, so
	// those bindings are switched off and vanish from the footer.
	if len(m.options) == 0 {
		m.keys.Up.SetEnabled(false)
		m.keys.Down.SetEnabled(false)
		m.keys.Select.SetEnabled(false)
	}
	return m
}

// Choice returns what the user selected once the program has finished.
func (m Model) Choice() Choice { return m.choice }

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
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.choice = Choice{Cancelled: true}
			m.done = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Select):
			if len(m.options) > 0 {
				o := m.options[m.cursor]
				m.choice = Choice{Kind: o.kind, Name: o.name}
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	f := frame.Frame{
		Theme:      m.theme,
		Width:      m.width,
		Height:     m.height,
		Tool:       "finops " + m.version,
		Breadcrumb: "auth",
		Keys:       m.help.View(m.keys),
		// Sparse, single-decision screen: centring reads as deliberate,
		// where top-alignment leaves a gap that looks like a cut-off screen.
		Center: true,
	}

	v := tea.NewView(f.Render(m.body()))
	// The TUI owns the terminal while it runs and restores it on exit.
	// Anything worth keeping comes from --format plain or --format json:
	// exploring and recording are different jobs (OVERHAUL.md D-A3).
	v.AltScreen = true
	return v
}

func (m Model) body() string {
	t := m.theme
	var b strings.Builder

	if len(m.options) == 0 {
		b.WriteString("  " + t.NegativeS().Render(theme.GlyphFail+" No AWS credentials found.") + "\n\n")
		b.WriteString("  " + t.DimS().Render("Set one up, then run finops again:") + "\n\n")
		b.WriteString("    " + t.AccentS().Render("aws configure sso") + t.DimS().Render("     IAM Identity Center (recommended)") + "\n")
		b.WriteString("    " + t.AccentS().Render("aws configure") + t.DimS().Render("         access keys") + "\n")
		return b.String()
	}

	b.WriteString("  No credentials resolved. Choose a source:\n\n")
	b.WriteString(m.sourceSummary())
	b.WriteString("\n")

	for i, o := range m.options {
		line := fmt.Sprintf("%-42s %s", o.label, o.hint)
		if i == m.cursor {
			b.WriteString("  " + t.SelectedS().Render(theme.GlyphSelected+" "+line) + "\n")
		} else {
			b.WriteString("    " + t.TextS().Render(strings.TrimRight(line, " ")) + "\n")
		}
	}
	return b.String()
}

// sourceSummary lists every source we looked for, present or not. Showing the
// misses matters: "not on EC2/ECS" is dim rather than red because unavailable
// is not an error, and a user debugging why nothing resolved needs to see
// what was checked.
func (m Model) sourceSummary() string {
	t := m.theme
	var b strings.Builder

	for _, s := range m.discovery.Sources {
		if s.Available {
			note := s.Detail
			if s.Note != "" {
				note += "  (" + s.Note + ")"
			}
			glyph := t.PositiveS().Render(theme.GlyphOK)
			line := fmt.Sprintf("%-24s %s", sourceLabel(s.Kind), note)
			if s.Expired() {
				b.WriteString("  " + glyph + " " + t.DimS().Render(fmt.Sprintf("%-24s ", sourceLabel(s.Kind))) +
					t.DimS().Render(s.Detail) + "  " + t.WarningS().Render("("+s.Note+")") + "\n")
				continue
			}
			b.WriteString("  " + glyph + " " + t.DimS().Render(line) + "\n")
		} else {
			line := fmt.Sprintf("%s %-24s %s", theme.GlyphFail, sourceLabel(s.Kind), s.Note)
			b.WriteString("  " + t.DimS().Render(line) + "\n")
		}
	}
	return b.String()
}

func sourceLabel(k core.Kind) string {
	switch k {
	case core.KindSSO:
		return "IAM Identity Center"
	case core.KindProfile:
		return "Shared profiles"
	case core.KindEnv:
		return "Environment variables"
	case core.KindAmbient:
		return "Instance metadata"
	case core.KindOIDC:
		return "Web identity"
	}
	return string(k)
}
