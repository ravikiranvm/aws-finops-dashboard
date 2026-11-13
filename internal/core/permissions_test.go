package core

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want PermissionState
	}{
		{"success", nil, PermGranted},

		// Every spelling AWS uses for "you may not do that".
		{"AccessDenied", &apiErr{"AccessDenied"}, PermDenied},
		{"AccessDeniedException", &apiErr{"AccessDeniedException"}, PermDenied},
		{"UnauthorizedOperation", &apiErr{"UnauthorizedOperation"}, PermDenied},
		{"AuthorizationError", &apiErr{"AuthorizationError"}, PermDenied},

		// Not being in an organization is a fact about the account, not a
		// missing permission — the call itself was allowed. Classifying this
		// as denied would tell users to fix an IAM policy that is already fine.
		{"not in an organization", &apiErr{"AWSOrganizationsNotInUseException"}, PermGranted},

		// Anything else is "could not verify", which is distinct from denied:
		// a throttle or a network blip must not be reported as a permissions
		// problem the user needs to fix.
		{"throttling", &apiErr{"ThrottlingException"}, PermError},
		{"service unavailable", &apiErr{"ServiceUnavailable"}, PermError},
		{"plain error", errors.New("dial tcp: connection refused"), PermError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.err); got != tt.want {
				t.Errorf("classify(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestPreflightCounts(t *testing.T) {
	p := Preflight{Permissions: []Permission{
		{Action: "a", State: PermGranted},
		{Action: "b", State: PermGranted},
		{Action: "c", State: PermDenied},
		{Action: "d", State: PermError},
		// Unchecked must not be counted either way: Cost Explorer is never
		// probed because the probe itself is billable, and reporting it as
		// granted or denied would both be lies.
		{Action: "e", State: PermUnchecked},
	}}

	granted, total := p.Counts()
	if granted != 2 {
		t.Errorf("granted = %d, want 2", granted)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4 (unchecked excluded)", total)
	}
}

func TestPreflightGranted(t *testing.T) {
	p := Preflight{Permissions: []Permission{
		{Action: "ec2:DescribeVolumes", State: PermGranted},
		{Action: "pricing:GetProducts", State: PermDenied},
		{Action: "ce:GetCostAndUsage", State: PermUnchecked},
	}}

	if !p.Granted("ec2:DescribeVolumes") {
		t.Error("granted action should report true")
	}
	if p.Granted("pricing:GetProducts") {
		t.Error("denied action must not report true")
	}
	// Unchecked is not granted: we genuinely do not know, and assuming yes
	// would mean silently attempting a billable call.
	if p.Granted("ce:GetCostAndUsage") {
		t.Error("unchecked action must not report as granted")
	}
	if p.Granted("nonexistent") {
		t.Error("unknown action must not report as granted")
	}
}

func TestPreflightDegradedNamesFeaturesNotActions(t *testing.T) {
	// A denial should surface as the capability the user loses, not as a raw
	// IAM string they then have to interpret.
	p := Preflight{Permissions: []Permission{
		{Action: "pricing:GetProducts", State: PermDenied, Enables: "savings estimates"},
		{Action: "ec2:DescribeVolumes", State: PermGranted, Enables: "EBS audits"},
		{Action: "budgets:DescribeBudgets", State: PermError, Enables: "budget alerts"},
	}}

	got := p.Degraded()
	if len(got) != 1 || got[0] != "savings estimates" {
		t.Errorf("Degraded() = %v, want [savings estimates]", got)
	}
}
