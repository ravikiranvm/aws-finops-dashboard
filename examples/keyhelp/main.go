// Demonstrates the Charm pattern that has no Ink equivalent:
// keybindings declared ONCE with their help text attached, from which the
// footer and the full help overlay render themselves.
package main

import (
	"fmt"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// keyMap declares every command once. Help text lives WITH the binding.
type keyMap struct {
	Account key.Binding
	Service key.Binding
	Period  key.Binding
	Metric  key.Binding
	Filter  key.Binding
	Drill   key.Binding
	Export  key.Binding
	Palette key.Binding
	Help    key.Binding
	Quit    key.Binding
}

// ShortHelp → the footer. FullHelp → the ? overlay. Both derived, never written twice.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Account, k.Service, k.Period, k.Filter, k.Drill, k.Export, k.Palette, k.Help}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Account, k.Service},         // grouping
		{k.Period, k.Metric, k.Filter}, // period & data
		{k.Drill, k.Export},            // navigation & output
		{k.Palette, k.Help, k.Quit},    // meta
	}
}

var keys = keyMap{
	Account: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "by account")),
	Service: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "by service")),
	Period:  key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "period")),
	Metric:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "cost metric")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Drill:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "drill down")),
	Export:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export")),
	Palette: key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "commands")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

type model struct {
	help     help.Model
	keys     keyMap
	orgMode  bool
	quitting bool
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.SetWidth(msg.Width)
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, m.keys.Account):
			// Disabling a binding removes it from the footer AND the overlay
			// automatically — help self-manages.
			m.orgMode = !m.orgMode
			m.keys.Metric.SetEnabled(m.orgMode)
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	body := "  AWS Cost · Jul 2026 · 14 accounts · net amortized\n\n" +
		"  prod-platform   $62,410\n  prod-data       $38,220\n  ml-training     $21,880\n\n" +
		dim.Render(fmt.Sprintf("  (press 'a' to toggle metric binding — currently enabled=%v)", m.orgMode)) + "\n\n"

	// One call renders the footer, or the full overlay, from the SAME declarations.
	return tea.NewView(body + "  " + m.help.View(m.keys) + "\n")
}

func main() {
	m := model{help: help.New(), keys: keys, orgMode: false}
	m.keys.Metric.SetEnabled(false) // start disabled to show self-managing help
	if _, err := tea.NewProgram(m).Run(); err != nil {
		panic(err)
	}
}
