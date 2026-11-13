// Package engine plans and runs audit scans.
package engine

import (
	"context"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// RegionScope is the set of regions a scan will sweep, and how it was arrived at.
type RegionScope struct {
	Regions []string
	// Source explains where the list came from, so the UI never presents a
	// guess as a decision: "detected", "flag", or "config".
	Source string
	// Enabled is every region enabled on the account, whether or not it holds
	// resources. Kept so the UI can say "4 of 17".
	Enabled []string
}

// DetectRegions works out which regions actually hold resources.
//
// The naive approach fails: ec2:DescribeRegions returns every *enabled* region,
// and nearly every account has 17–30 enabled by default. Scanning all of them
// is the fan-out problem, not a solution.
//
// Cost Explorer with GroupBy=REGION would give regions with real spend, but it
// costs ~$0.01 per call and the audit is meant to be free.
//
// So: list enabled regions (one free call), then probe each in parallel with a
// tiny, free DescribeVolumes. Regions holding nothing drop out. Twenty parallel
// calls land in a few hundred milliseconds.
//
// The probe uses EC2 as a proxy for "has resources here", which suits the v3
// check set — volumes, snapshots, stopped instances, Elastic IPs and gp2→gp3
// are all EC2-family, and budgets are global. When S3 checks land the probe
// will need a second signal.
func DetectRegions(ctx context.Context, cfg aws.Config) (RegionScope, error) {
	client := ec2.NewFromConfig(cfg)

	out, err := client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		// Without the region list we cannot fan out at all, but the region the
		// credentials already resolved to is always usable. Degrade to that
		// rather than failing the whole scan.
		if cfg.Region != "" {
			return RegionScope{Regions: []string{cfg.Region}, Source: "credentials", Enabled: []string{cfg.Region}}, nil
		}
		return RegionScope{}, err
	}

	enabled := make([]string, 0, len(out.Regions))
	for _, r := range out.Regions {
		if name := aws.ToString(r.RegionName); name != "" {
			enabled = append(enabled, name)
		}
	}
	sort.Strings(enabled)

	active := probeRegions(ctx, cfg, enabled)
	if len(active) == 0 {
		// An account with no EC2 anywhere is legitimate. Fall back to the
		// resolved region so checks that are not EC2-shaped still run.
		if cfg.Region != "" {
			active = []string{cfg.Region}
		}
	}

	return RegionScope{Regions: active, Source: "detected", Enabled: enabled}, nil
}

// probeRegions asks each region whether it holds any EBS volumes, concurrently.
//
// The bound is generous on purpose. These are small, independent, free calls to
// distant endpoints, so latency dominates and the batch is only as fast as its
// widest fan-out: at 8-way concurrency an 18-region account takes three
// sequential rounds and ~1.9s, where going wide finishes in one round.
func probeRegions(ctx context.Context, cfg aws.Config, regions []string) []string {
	const maxConcurrent = 24

	var (
		mu     sync.Mutex
		active []string
		wg     sync.WaitGroup
		sem    = make(chan struct{}, maxConcurrent)
	)

	for _, region := range regions {
		wg.Add(1)
		go func(region string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			regional := cfg.Copy()
			regional.Region = region

			out, err := ec2.NewFromConfig(regional).DescribeVolumes(ctx,
				&ec2.DescribeVolumesInput{MaxResults: aws.Int32(5)})
			if err != nil {
				// A region we cannot reach is not a region with resources.
				// Opt-in regions that are enabled but unreachable land here.
				return
			}
			if len(out.Volumes) == 0 {
				return
			}

			mu.Lock()
			active = append(active, region)
			mu.Unlock()
		}(region)
	}
	wg.Wait()

	sort.Strings(active)
	return active
}
