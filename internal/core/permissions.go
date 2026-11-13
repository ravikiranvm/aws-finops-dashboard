package core

import (
	"context"
	"errors"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

// PermissionState is the outcome of probing one IAM action.
type PermissionState string

const (
	PermGranted PermissionState = "granted"
	PermDenied  PermissionState = "denied"
	// PermUnchecked means we deliberately did not probe. Cost Explorer calls
	// are billed per request (~$0.01), so verifying them would charge the user
	// every time they run `finops auth`. Saying so is better than either
	// silently charging them or silently claiming we know.
	PermUnchecked PermissionState = "unchecked"
	PermError     PermissionState = "error"
)

// Permission is one probed IAM action and what it gates.
type Permission struct {
	Action string
	State  PermissionState
	// Enables names the feature that stops working without it, so a denial
	// reads as a consequence rather than a bare IAM string.
	Enables string
	Note    string
}

// Preflight is the result of checking what these credentials can actually do.
//
// Probing up front turns "the audit returned nothing" into "you are missing
// ec2:DescribeVolumes" (OVERHAUL.md D-B4).
type Preflight struct {
	Permissions []Permission
}

// Granted reports whether a specific action probed as allowed.
func (p Preflight) Granted(action string) bool {
	for _, perm := range p.Permissions {
		if perm.Action == action {
			return perm.State == PermGranted
		}
	}
	return false
}

// Counts summarises the probe for the status line.
func (p Preflight) Counts() (granted, total int) {
	for _, perm := range p.Permissions {
		if perm.State == PermUnchecked {
			continue
		}
		total++
		if perm.State == PermGranted {
			granted++
		}
	}
	return granted, total
}

// Degraded lists the features unavailable because a permission was denied.
func (p Preflight) Degraded() []string {
	var out []string
	for _, perm := range p.Permissions {
		if perm.State == PermDenied && perm.Enables != "" {
			out = append(out, perm.Enables)
		}
	}
	return out
}

// CheckPermissions probes the IAM actions the tool depends on.
//
// Only free calls are probed, and every probe is shaped to return as little as
// possible — small MaxResults, no pagination. Cost Explorer is never probed.
// accountID comes from the already-resolved identity so this costs no extra
// STS call.
func CheckPermissions(ctx context.Context, cfg aws.Config, accountID string) Preflight {
	type probe struct {
		action  string
		enables string
		run     func(context.Context) error
	}

	ec2c := ec2.NewFromConfig(cfg)
	orgc := organizations.NewFromConfig(cfg)
	budc := budgets.NewFromConfig(cfg)
	// The Price List API is only served from a few regions; us-east-1 is
	// always one of them.
	pricec := pricing.NewFromConfig(cfg, func(o *pricing.Options) { o.Region = "us-east-1" })

	probes := []probe{
		{"ec2:DescribeRegions", "region discovery", func(ctx context.Context) error {
			_, err := ec2c.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
			return err
		}},
		{"ec2:DescribeVolumes", "EBS audits", func(ctx context.Context) error {
			_, err := ec2c.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{MaxResults: aws.Int32(5)})
			return err
		}},
		{"ec2:DescribeInstances", "EC2 audits", func(ctx context.Context) error {
			_, err := ec2c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{MaxResults: aws.Int32(5)})
			return err
		}},
		{"ec2:DescribeAddresses", "Elastic IP audit", func(ctx context.Context) error {
			_, err := ec2c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
			return err
		}},
		{"organizations:ListAccounts", "organization mode", func(ctx context.Context) error {
			_, err := orgc.ListAccounts(ctx, &organizations.ListAccountsInput{MaxResults: aws.Int32(1)})
			return err
		}},
		{"budgets:DescribeBudgets", "budget alerts", func(ctx context.Context) error {
			_, err := budc.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
				AccountId:  aws.String(accountID),
				MaxResults: aws.Int32(1),
			})
			return err
		}},
		{"pricing:GetProducts", "savings estimates", func(ctx context.Context) error {
			_, err := pricec.DescribeServices(ctx, &pricing.DescribeServicesInput{MaxResults: aws.Int32(1)})
			return err
		}},
	}

	results := make([]Permission, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(i int, p probe) {
			defer wg.Done()
			results[i] = Permission{Action: p.action, Enables: p.enables, State: classify(p.run(ctx))}
		}(i, p)
	}
	wg.Wait()

	// Cost Explorer is listed but never probed: the probe itself is billable.
	results = append(results, Permission{
		Action:  "ce:GetCostAndUsage",
		State:   PermUnchecked,
		Enables: "cost dashboard",
		Note:    "not probed — Cost Explorer requests are billed",
	})

	return Preflight{Permissions: results}
}

func classify(err error) PermissionState {
	if err == nil {
		return PermGranted
	}
	var api interface{ ErrorCode() string }
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation",
			"AuthorizationError", "AccessDeniedError":
			return PermDenied
		case "AWSOrganizationsNotInUseException":
			// Not an organization is a fact about the account, not a missing
			// permission — the call itself was allowed.
			return PermGranted
		}
	}
	return PermError
}
