package audit

import "testing"

func waste(id, svc string, apis ...string) Check {
	return Check{
		ID: id, Name: id, Service: svc,
		Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs: apis, DefaultEnabled: true,
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		check   Check
		wantErr bool
	}{
		{"valid waste check", waste("a", "EBS", "ec2:DescribeVolumes"), false},
		{"missing ID", Check{Name: "x", Service: "EBS", Category: CategoryWaste,
			Basis: BasisExact, Magnitude: MagnitudeSavings, APIs: []string{"x"}}, true},
		{"no APIs declared", Check{ID: "a", Name: "a", Service: "EBS",
			Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings}, true},
		{"waste without savings magnitude", Check{ID: "a", Name: "a", Service: "EBS",
			Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeCount,
			APIs: []string{"x"}}, true},
		{"waste with no basis", Check{ID: "a", Name: "a", Service: "EBS",
			Category: CategoryWaste, Basis: BasisNone, Magnitude: MagnitudeSavings,
			APIs: []string{"x"}}, true},
		{"allocation must be unallocated", Check{ID: "a", Name: "a", Service: "EC2",
			Category: CategoryAllocation, Basis: BasisNone, Magnitude: MagnitudeSavings,
			APIs: []string{"x"}}, true},
		{"valid allocation", Check{ID: "a", Name: "a", Service: "EC2",
			Category: CategoryAllocation, Basis: BasisNone, Magnitude: MagnitudeUnallocated,
			APIs: []string{"x"}}, false},
		{"policy with a basis", Check{ID: "a", Name: "a", Service: "S3",
			Category: CategoryPolicy, Basis: BasisExact, Magnitude: MagnitudeCount,
			APIs: []string{"x"}}, true},
		{"valid policy", Check{ID: "a", Name: "a", Service: "S3",
			Category: CategoryPolicy, Basis: BasisNone, Magnitude: MagnitudeCount,
			CountLabel: "violating", APIs: []string{"x"}}, false},
		{"count without a label", Check{ID: "a", Name: "a", Service: "S3",
			Category: CategoryPolicy, Basis: BasisNone, Magnitude: MagnitudeCount,
			APIs: []string{"x"}}, true},
		{"unknown category", Check{ID: "a", Name: "a", Service: "S3",
			Category: "nonsense", Magnitude: MagnitudeCount, APIs: []string{"x"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequiredAPIsDeduplicates(t *testing.T) {
	// Two checks sharing an API must produce one call, not two. This is the
	// property that makes "run all" cheaper than running categories serially.
	checks := []Check{
		waste("unattached", "EBS", "ec2:DescribeVolumes"),
		waste("gp2-to-gp3", "EBS", "ec2:DescribeVolumes"),
		waste("idle-nat", "NATGW", "ec2:DescribeNatGateways", "cloudwatch:GetMetricData"),
	}

	got := RequiredAPIs(checks)
	want := []string{"cloudwatch:GetMetricData", "ec2:DescribeNatGateways", "ec2:DescribeVolumes"}

	if len(got) != len(want) {
		t.Fatalf("RequiredAPIs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RequiredAPIs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
