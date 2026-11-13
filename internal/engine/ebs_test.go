package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
)

func vol(id string, sizeGB int32, volType string, tags map[string]string) ec2types.Volume {
	v := ec2types.Volume{
		VolumeId:   aws.String(id),
		Size:       aws.Int32(sizeGB),
		VolumeType: ec2types.VolumeType(volType),
		CreateTime: aws.Time(time.Now().Add(-60 * 24 * time.Hour)),
	}
	for k, val := range tags {
		v.Tags = append(v.Tags, ec2types.Tag{Key: aws.String(k), Value: aws.String(val)})
	}
	return v
}

// inv builds a one-region inventory for the pure evaluators.
func inv(vols ...ec2types.Volume) Inventory {
	return Inventory{Account: "123456789012", Region: "us-east-1", Volumes: vols}
}

func gp3Pricer() StaticPricer {
	return StaticPricer{PerGBMonth: map[string]float64{"gp3": 0.08, "gp2": 0.10}}
}

func TestUnattachedEBSComputesExactSavings(t *testing.T) {
	v := vol("vol-aaa", 500, "gp3", nil)
	v.State = ec2types.VolumeStateAvailable

	got := UnattachedEBS{}.Evaluate(context.Background(), inv(v), gp3Pricer())
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}

	f := got[0]
	if want := 500 * 0.08; f.Magnitude.USDPerMonth != want {
		t.Errorf("savings = %.2f, want %.2f", f.Magnitude.USDPerMonth, want)
	}
	if f.Magnitude.Basis != audit.BasisExact {
		t.Errorf("basis = %q, want exact — this is arithmetic, not estimation", f.Magnitude.Basis)
	}
	if f.Magnitude.Kind != audit.MagnitudeSavings {
		t.Errorf("kind = %q, want savings", f.Magnitude.Kind)
	}
	if !f.Magnitude.Priced() {
		t.Error("finding should report as priced")
	}

	// Evidence is the honesty mechanism: the arithmetic has to be inspectable.
	joined := strings.Join(f.Evidence, " ")
	for _, want := range []string{"500 GB", "0.0800", "40.00"} {
		if !strings.Contains(joined, want) {
			t.Errorf("evidence missing %q: %v", want, f.Evidence)
		}
	}
}

// Pricing being unavailable must not lose the finding. Eight unattached volumes
// are worth reporting without dollars; showing $0 would be a lie.
func TestUnattachedEBSSurvivesMissingPrice(t *testing.T) {
	v := vol("vol-aaa", 500, "io2", nil) // no price for io2 in the fake
	v.State = ec2types.VolumeStateAvailable

	got := UnattachedEBS{}.Evaluate(context.Background(), inv(v), gp3Pricer())
	if len(got) != 1 {
		t.Fatalf("got %d findings, want the volume reported anyway", len(got))
	}
	if got[0].Magnitude.Priced() {
		t.Error("must not claim a price it does not have")
	}
	if got[0].Magnitude.USDPerMonth != 0 {
		t.Errorf("USDPerMonth = %v, want 0", got[0].Magnitude.USDPerMonth)
	}
	if !strings.Contains(strings.Join(got[0].Evidence, " "), "price unavailable") {
		t.Errorf("evidence should say why there is no price: %v", got[0].Evidence)
	}
}

func TestUnattachedEBSOwnerFromTags(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
		want string
	}{
		{"Owner", map[string]string{"Owner": "team-api"}, "team-api"},
		{"lowercase team", map[string]string{"team": "platform"}, "platform"},
		{"CostCenter", map[string]string{"CostCenter": "cc-42"}, "cc-42"},
		{"unrelated tags only", map[string]string{"Name": "scratch"}, ""},
		{"no tags", nil, ""},
		{"empty value is not an owner", map[string]string{"Owner": ""}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := vol("vol-a", 10, "gp3", tt.tags)
			v.State = ec2types.VolumeStateAvailable
			got := UnattachedEBS{}.Evaluate(context.Background(), inv(v), gp3Pricer())
			if got[0].Owner != tt.want {
				t.Errorf("owner = %q, want %q", got[0].Owner, tt.want)
			}
		})
	}
}

func TestFingerprintIsStableAndDistinct(t *testing.T) {
	// A volume growing from 100 GB to 200 GB is the same finding, not a new one
	// — otherwise a suppression would silently stop applying.
	a := audit.NewFingerprint("ebs-unattached", "123", "us-east-1", "vol-aaa")
	if a != audit.NewFingerprint("ebs-unattached", "123", "us-east-1", "vol-aaa") {
		t.Error("fingerprint should be deterministic")
	}
	if a == audit.NewFingerprint("ebs-unattached", "123", "eu-west-1", "vol-aaa") {
		t.Error("same resource id in a different region must not collide")
	}
	if a == audit.NewFingerprint("ebs-gp2-to-gp3", "123", "us-east-1", "vol-aaa") {
		t.Error("different checks on the same resource must not collide")
	}
}
