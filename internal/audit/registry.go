package audit

import (
	"fmt"
	"sort"
)

// registry holds every declared check, keyed by ID.
var registry = map[string]Check{}

// Register adds a check declaration. It panics on an invalid or duplicate
// declaration: these are programmer errors that should fail at startup rather
// than produce a nonsensical catalogue at runtime.
func Register(c Check) {
	if err := c.Validate(); err != nil {
		panic(fmt.Sprintf("audit.Register: %v", err))
	}
	if _, dup := registry[c.ID]; dup {
		panic(fmt.Sprintf("audit.Register: duplicate check ID %q", c.ID))
	}
	registry[c.ID] = c
}

// All returns every declared check, ordered by category then service then name
// so the catalogue is stable across runs.
func All() []Check {
	out := make([]Check, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sortChecks(out)
	return out
}

// ByCategory returns checks grouped for the catalogue view, in display order.
// Categories with no checks are omitted.
func ByCategory() map[Category][]Check {
	grouped := map[Category][]Check{}
	for _, c := range All() {
		grouped[c.Category] = append(grouped[c.Category], c)
	}
	return grouped
}

// Get returns a check by ID.
func Get(id string) (Check, bool) {
	c, ok := registry[id]
	return c, ok
}

// RequiredAPIs returns the deduplicated set of AWS API calls needed to run the
// given checks. This is what makes running everything cheaper than running each
// category separately: several checks share ec2:DescribeVolumes, and the
// planner fetches it once.
func RequiredAPIs(checks []Check) []string {
	seen := map[string]bool{}
	for _, c := range checks {
		for _, api := range c.APIs {
			seen[api] = true
		}
	}
	out := make([]string, 0, len(seen))
	for api := range seen {
		out = append(out, api)
	}
	sort.Strings(out)
	return out
}

func sortChecks(cs []Check) {
	rank := map[Category]int{}
	for i, c := range Order {
		rank[c] = i
	}
	sort.Slice(cs, func(i, j int) bool {
		if rank[cs[i].Category] != rank[cs[j].Category] {
			return rank[cs[i].Category] < rank[cs[j].Category]
		}
		if cs[i].Service != cs[j].Service {
			return cs[i].Service < cs[j].Service
		}
		return cs[i].Name < cs[j].Name
	})
}
