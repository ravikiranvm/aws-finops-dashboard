package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func main() {
	base := strings.Join([]string{
		"  AWS Cost · Jul 2026 · 14 accounts",
		"",
		"  $147,206  month to date",
		"  $198,400  forecast",
		"",
		"  ACCOUNT           MTD      PREV",
		"  prod-platform  $62,410  $48,900",
		"  prod-data      $38,220  $37,100",
		"  ml-training    $21,880   $9,440",
		"  prod-edge      $12,090  $12,650",
		"  staging         $6,340   $6,180",
		"  data-lake       $2,410   $3,980",
		"",
		"  [p]eriod [/]filter  :  ?",
	}, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render("period\n▸ month to date\n  last month\n  last 7 days")

	fmt.Printf("base: %dx%d   box: %dx%d\n\n",
		lipgloss.Width(base), lipgloss.Height(base),
		lipgloss.Width(box), lipgloss.Height(box))

	fmt.Println("--- C: NewCompositor(base, picker at X4 Y5 Z1).Render() ---")
	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(4).Y(5).Z(1).ID("picker"),
	)
	fmt.Println(comp.Render())

	fmt.Println("--- hit test at (10,7) ---")
	fmt.Println("hit id:", comp.Hit(10, 7).ID())
	fmt.Println("hit id at (1,1):", comp.Hit(1, 1).ID())
}
