package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
)

// Live tests hit real AWS. They are skipped unless FINOPS_LIVE=1, so
// `go test ./...` stays offline, fast, and dependency-free — the property
// D-X8 relies on so that anyone (or any agent) can run the suite without setup.
//
//	FINOPS_LIVE=1 go test ./internal/engine/ -run Live -v
func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("FINOPS_LIVE") == "" {
		t.Skip("set FINOPS_LIVE=1 to run against real AWS")
	}
}

func TestDetectRegionsLive(t *testing.T) {
	requireLive(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, cfg, err := core.NewResolver(core.OSEnv{}).Resolve(ctx, core.Options{})
	if err != nil {
		t.Fatalf("resolve credentials: %v", err)
	}

	start := time.Now()
	scope, err := DetectRegions(ctx, cfg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("DetectRegions: %v", err)
	}

	t.Logf("enabled regions:  %d", len(scope.Enabled))
	t.Logf("active regions:   %v", scope.Regions)
	t.Logf("source:           %s", scope.Source)
	t.Logf("elapsed:          %s", elapsed.Round(10*time.Millisecond))

	if len(scope.Regions) == 0 {
		t.Error("detected no regions at all; expected at least the resolved region")
	}
	// The whole point is to narrow the sweep. If detection returns everything
	// enabled, it has bought us nothing and the scan will be slow.
	if len(scope.Enabled) > 5 && len(scope.Regions) == len(scope.Enabled) {
		t.Errorf("detection did not narrow anything: %d active of %d enabled",
			len(scope.Regions), len(scope.Enabled))
	}
}

// The Price List API is the one dependency where guessing the request shape is
// expensive: it historically required region display names ("Asia Pacific
// (Singapore)") rather than codes, and returns opaque JSON with no typed
// accessor. This confirms regionCode works and that the traversal finds a price.
func TestEBSPricingLive(t *testing.T) {
	requireLive(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, cfg, err := core.NewResolver(core.OSEnv{}).Resolve(ctx, core.Options{})
	if err != nil {
		t.Fatalf("resolve credentials: %v", err)
	}

	p := NewAWSPricer(cfg)
	for _, tc := range []struct{ region, volType string }{
		{"us-east-1", "gp3"},
		{"us-east-1", "gp2"},
		{"ap-southeast-1", "gp3"},
		{"ap-southeast-1", "gp2"},
	} {
		price, err := p.EBSPerGBMonth(ctx, tc.region, tc.volType)
		if err != nil {
			t.Errorf("%s/%s: %v", tc.region, tc.volType, err)
			continue
		}
		t.Logf("%-16s %-4s  $%.4f per GB-month", tc.region, tc.volType, price)
		// Sanity bounds: EBS is cents per GB-month everywhere. A value outside
		// this range means we parsed the wrong price dimension.
		if price < 0.01 || price > 1.0 {
			t.Errorf("%s/%s price $%.4f is outside a plausible range", tc.region, tc.volType, price)
		}
	}
}

// The vertical slice: resolve credentials, detect regions, scan each one for
// unattached volumes, price them. This is the first point the tool does the
// thing it exists to do.
func TestUnattachedEBSLive(t *testing.T) {
	requireLive(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, cfg, err := core.NewResolver(core.OSEnv{}).Resolve(ctx, core.Options{})
	if err != nil {
		t.Fatalf("resolve credentials: %v", err)
	}

	scope, err := DetectRegions(ctx, cfg)
	if err != nil {
		t.Fatalf("detect regions: %v", err)
	}
	t.Logf("account %s · regions %v", id.AccountID, scope.Regions)

	pricer := NewAWSPricer(cfg)
	total := 0.0
	count := 0

	for _, region := range scope.Regions {
		regional := cfg.Copy()
		regional.Region = region

		client := ec2.NewFromConfig(regional)

		// Fetch once, evaluate many — the planner's dedup, exercised for real.
		inventory, ierr := fetchInventory(ctx, client,
			planNeeds([]string{"ec2:DescribeVolumes"}), id.AccountID, region)
		if ierr != nil {
			t.Errorf("%s inventory: %v", region, ierr)
			continue
		}
		t.Logf("%s: %d volumes fetched in one call", region, len(inventory.Volumes))

		var findings []audit.Finding
		findings = append(findings, UnattachedEBS{}.Evaluate(ctx, inventory, pricer)...)
		findings = append(findings, GP2ToGP3{}.Evaluate(ctx, inventory, pricer)...)

		for _, f := range findings {
			count++
			total += f.Magnitude.USDPerMonth
			t.Logf("  %s", f.Title)
			for _, e := range f.Evidence {
				t.Logf("      %s", e)
			}
			if f.Owner != "" {
				t.Logf("      owner: %s", f.Owner)
			}
		}
	}

	t.Logf("")
	t.Logf("%d findings · $%.2f/month", count, total)
	if count == 0 {
		t.Log("(none found — a clean account is a valid result, not a failure)")
	}
}
