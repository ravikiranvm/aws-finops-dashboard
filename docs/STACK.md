# Stack facts — verified, version-stamped

**Purpose.** Charm v2 shipped 2026-07-03. Model training data is largely **v1-shaped**, and
v1 idioms in this stack *compile and misbehave* rather than failing loudly. This file records
facts that have been **verified against installed source**, not recalled.

**Rules**
1. Every entry carries the exact version, the date verified, and the command to re-verify.
2. Weight entries toward things that **type-check but do the wrong thing** — those are invisible
   to the compiler and to training data.
3. If you need an API that isn't here, **verify it before use** (`go doc`, or read `$GOMODCACHE`),
   then add it here. Do not rely on recall.
4. Dependency upgrades must update this file in the same commit.

---

## charm.land/bubbletea/v2 @ v2.0.8 — verified 2026-07-25

**`Model.View()` returns `tea.View`, not `string`.** The v1 idiom does not compile.

```go
func (m model) View() tea.View { return tea.NewView(someString) }
```
`tea.View` is a struct with `Content string` and an optional `OnMouse` handler.
Verify: `go doc charm.land/bubbletea/v2.Model`

**Inline is the default.** Alt-screen is opt-in via a program option. Inline output mixes with
normal terminal output, so scrollback is preserved — this is what `D-A3` depends on.
There is documented friction printing to scrollback *across* inline↔alt-screen transitions
(`tea.Println` vs `tea.PrintlnRaw`, `insertAbove`), which is why we never switch modes.

**Key handling:** `case tea.KeyPressMsg:` then `msg.String()`, or `key.Matches(msg, binding)`.

⚠️ **`key.WithKeys` takes key *names*, not characters.** Space is `"space"`, not `" "` — a
literal space silently never matches, and the binding appears in the help while doing nothing.
Likewise `"enter"`, `"esc"`, `"up"`, `"tab"`. Verified: `tea.KeyPressMsg{Code: ' '}.String()`
returns `"space"`.

### ⚠️ Alt-screen is a View field, not a program option

`tea.WithAltScreen()` does **not exist** in v2 — the v1 idiom fails to compile. Terminal state
is declared by the view, so a model can toggle it per frame:

```go
v := tea.NewView(content)
v.AltScreen = true
return v
```
`tea.View` also carries `Cursor`, `WindowTitle`, `BackgroundColor`, `ForegroundColor`,
`MouseMode`, `ProgressBar`, `ReportFocus`, and `KeyboardEnhancements` — all declarative.
Verify: `go doc charm.land/bubbletea/v2.View`

**Terminal background arrives as a message**, not a query:
```go
case tea.BackgroundColorMsg:
    m.isDark = msg.IsDark()
```
Delivered through `Update` on start and whenever it changes.

**Mouse is first-class:** `MouseMsg` interface plus `MouseClickMsg`, `MouseMotionMsg`,
`MouseReleaseMsg`, `MouseWheelMsg`, `MouseButton`, `MouseMode`.
Verify: `go doc charm.land/bubbletea/v2 | grep -i mouse`

---

## charm.land/lipgloss/v2 @ v2.0.5 — verified 2026-07-25

### ⚠️ `Canvas.Compose(*Layer)` type-checks and silently does the wrong thing

`Layer.Draw()` **ignores the layer's own X/Y and all of its child layers** — it blits its content
into whatever area it is given. So `Canvas.Compose(layer)` drops positioning, and a second
`Compose` overwrites the first at origin rather than layering.

```go
// WRONG — compiles, renders only the last layer, at 0,0
c := lipgloss.NewCanvas(w, h)
c.Compose(lipgloss.NewLayer(base))
c.Compose(lipgloss.NewLayer(box).X(4).Y(9).Z(1))

// WRONG — compiles, child layer never appears
c.Compose(lipgloss.NewLayer(base, lipgloss.NewLayer(box).X(4).Y(9)))

// CORRECT — Compositor flattens the hierarchy, computes absolute
// positions, and sorts by z-index
comp := lipgloss.NewCompositor(
    lipgloss.NewLayer(base),
    lipgloss.NewLayer(box).X(4).Y(9).Z(1).ID("picker"),
)
out := comp.Render()
```
`Compositor.Hit(x, y) LayerHit` gives z-ordered hit testing by layer ID — verified working.
Verify: read `$GOMODCACHE/charm.land/lipgloss/v2@v2.0.5/layer.go`

### `AdaptiveColor` is gone; adaptive theming is not

v2 removed the `AdaptiveColor` *type* and replaced it with a message-driven equivalent:

```go
lightDark := lipgloss.LightDark(isDark)          // isDark from tea.BackgroundColorMsg
style := lipgloss.NewStyle().Foreground(lightDark(lightColor, darkColor))
```
Verify: `go doc charm.land/lipgloss/v2.LightDark`

### There is no flexbox

The entire layout surface is `JoinHorizontal`, `JoinVertical`, `Place`, `PlaceHorizontal`,
`PlaceVertical`. Positional composition only. **Responsive width handling is arithmetic we own**
— budget for it explicitly rather than discovering it late.

---

## charm.land/bubbles/v2 @ v2.1.1 — verified 2026-07-26

**14 first-party components:** textarea, list, viewport, table, textinput, filepicker, progress,
paginator, help, cursor, spinner, timer, stopwatch, key.

**`key` + `help` — declare a binding once, both views derive from it.** This is the mechanism
behind `D-A4`; do not hand-write footer strings.

```go
Period: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "period"))
// keyMap implements ShortHelp() []key.Binding  → footer
//                   FullHelp() [][]key.Binding → ? overlay
help.Model.View(keyMap)
```
Options are `WithKeys`, `WithHelp`, `WithDisabled`. `binding.SetEnabled(false)` removes it from
**both** views automatically — the docs state help "should self-manage".

**`textinput` has a suggestion engine:** `SetSuggestions`, `AvailableSuggestions`,
`MatchedSuggestions`, `CurrentSuggestion`, `CurrentSuggestionIndex`.

**`list` has ranked fuzzy filtering built in:** `FilterFunc`, an embedded `FilterInput
textinput.Model`, `FilterState`, `SetFilterText`, `IsFiltered`, plus `DefaultFilter` /
`UnsortedFilter` returning ranked results. Together with `textinput` this is most of a
command palette without writing one.

**No tabs component, no focus manager.** Both are hand-rolled.

---

## aws-sdk-go-v2 — verified 2026-07-27

### ⚠️ Budgets amounts are `*string`, not numbers

`budgets/types.Spend.Amount` is a `*string`. Every amount has to be parsed, and a parse failure
must not fall through to zero — v2's Python `float(...)` would have turned an unreadable amount
into `0.0`, which reads as a healthy budget.

```go
v, err := strconv.ParseFloat(*s.Amount, 64)   // never ignore err
```

Also on `types.Budget`:

- `BudgetLimit` is **nil** for RI and Savings Plans utilisation/coverage budgets. Dereferencing it
  is a panic on any account that uses them.
- `CalculatedSpend` is nil on a freshly created budget, and `ForecastedSpend` is nil for a budget
  in its first period. Absent is not zero.
- `Spend.Unit` is **not always USD** — a usage budget is denominated in GB, requests, or hours.
  Printing a `$` in front of it fabricates a cost figure.

### Budgets is a global service

One endpoint, homed in `us-east-1`, returning the same account-wide answer whatever region the
config names. Running it in a per-region loop multiplies every finding by the region count. This
is why `audit.Check` carries a `Scope` and the planner splits its fan-out on it.

## Testing — teatest/v2 + golden — verified 2026-07-26

Charm ships first-party test helpers; **do not hand-roll a pty harness.**

```go
tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(88, 40))
tm.Send(tea.KeyPressMsg{ /* ... */ })
teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte("Total")) })
golden.RequireEqual(t, out)          // -update regenerates
```

- `github.com/charmbracelet/x/exp/teatest/v2` — `NewTestModel`, `WithInitialTermSize`,
  `WithProgramOptions`, `WaitFor`, `RequireEqualOutput`
- `github.com/charmbracelet/x/exp/golden` — `RequireEqual`, `RequireEqualEscape`

`WithInitialTermSize` removes the classic footgun: **a pty with no window size makes a TUI see
0×0 and render nothing at all** (observed during the spike — output was pure erase sequences).

`WaitFor` replaces sleep-based waiting, so tests on streaming output are deterministic.

**Never reconstruct frames by stripping ANSI.** Bubble Tea v2's renderer emits *incremental*,
cursor-positioned updates; concatenating stripped output yields plausible-looking garbage
(observed: `staging$6,340`, stray tabs) that reads as a real bug but isn't. `teatest` captures
program output directly, which sidesteps this. If true terminal emulation is ever needed,
`github.com/charmbracelet/x/vt` exists.

### ⚠️ Golden-frame the final model, not `FinalOutput`

`FinalOutput` returns the **byte stream**, which is a history of edits, not a picture of the
screen. Two consequences, both hit on 2026-07-27:

1. **Negative assertions silently test history.** `bytes.Contains(out, "vol-…000")` was true for a
   row that had scrolled away twenty keystrokes earlier. The assertion passes or fails on what was
   once drawn, not on what is on screen.
2. **Snapshots of it are flaky.** How the renderer coalesces incremental updates depends on
   timing, so the same test produced different goldens run to run — identical final screen, one
   frame's worth of different escape sequences.

Take both assertions and snapshots from the rendered final model instead:

```go
m := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(Model)
frame := m.View().Content            // deterministic; the frame a person sees
golden.RequireEqual(t, []byte(frame))
```

Keep `WaitFor` on `tm.Output()` for synchronisation — the stream is the right thing to watch for
"has the async scan landed yet", just not to assert against.

### ⚠️ Async commands race the keys that follow them

`tm.Send` does not block. Keys aimed at a screen that a `tea.Cmd` has not opened yet land on the
previous screen, where they are usually ignored — an intermittent failure that reads as a
navigation bug. Gate on the output first:

```go
tm.Send(tea.KeyPressMsg{Code: 'r'})   // starts an async scan
teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
    return bytes.Contains(b, []byte("exact basis"))   // results screen is up
})
tm.Send(tea.KeyPressMsg{Code: 'j'})   // now safe
```

⚠️ `teatest/v2` was first published **2026-07-26** — very fresh. Verify its API against source
before use, same as the rest of the v2 line.
