// Package exitcode defines the process exit taxonomy.
//
// Exit codes are part of the agent and CI contract (OVERHAUL.md R-C22): a
// caller must be able to distinguish "the tool failed" from "the tool worked
// and found something". Never add a code without documenting it here and in
// `finops schema`.
package exitcode

const (
	// OK: ran successfully, nothing actionable found.
	OK = 0
	// Error: an unexpected failure.
	Error = 1
	// Usage: bad flags or arguments.
	Usage = 2
	// Auth: no credentials, or they expired.
	Auth = 3
	// Permission: credentials resolved but lack a required IAM action.
	Permission = 4
	// NoData: the query succeeded but returned nothing (e.g. a tag filter
	// matching no resources). Distinct from OK so scripts can tell them apart.
	NoData = 5
	// Findings: the run succeeded and findings exist. Lets `finops audit`
	// act as a CI guardrail without parsing output.
	Findings = 10
)
