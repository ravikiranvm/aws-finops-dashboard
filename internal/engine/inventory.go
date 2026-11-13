package engine

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
)

// Inventory is everything fetched for one account+region, shared by every check
// that runs against it.
//
// This is the point of having a planner. Both EBS checks need
// ec2:DescribeVolumes: unattached wants status=available, gp2→gp3 wants
// volumeType=gp2. Letting each fetch its own copy would double the API calls
// and the wall clock for no benefit. Instead the planner collects what the
// selected checks need, fetches each thing once, and evaluators filter in
// memory.
//
// It also makes "run all" genuinely cheaper than running checks one at a time —
// the opposite of what users assume — which is worth surfacing in the estimate.
type Inventory struct {
	Account string
	Region  string

	// Volumes is every EBS volume in the region, unfiltered. Checks narrow it
	// themselves.
	Volumes []ec2types.Volume

	// Budgets is every budget in the account. Account-scoped, so it is fetched
	// once per account and Region is "global" on the inventory that carries it.
	Budgets []budgettypes.Budget
}

// needs describes which API calls a set of checks requires, deduplicated.
type needs struct {
	volumes bool
	budgets bool
}

// planNeeds works out what to fetch from the checks' own declarations, rather
// than from a hardcoded list here. Adding a check that needs volumes requires no
// change to the planner (OVERHAUL.md D-X9).
func planNeeds(apis []string) needs {
	var n needs
	for _, api := range apis {
		switch api {
		case "ec2:DescribeVolumes":
			n.volumes = true
		case "budgets:DescribeBudgets":
			n.budgets = true
		}
	}
	return n
}

// fetchAccountInventory retrieves the account-wide things the planned checks
// need. Called once per account rather than once per region, because a global
// endpoint returns the same answer from every region and reporting it per
// region would multiply every finding by the region count.
func fetchAccountInventory(ctx context.Context, api BudgetsAPI, n needs, account string) (Inventory, error) {
	inv := Inventory{Account: account, Region: "global"}

	if n.budgets {
		bs, err := allBudgets(ctx, api, account)
		inv.Budgets = bs
		if err != nil {
			return inv, err
		}
	}
	return inv, nil
}

// fetchInventory retrieves everything the planned checks need for one region.
//
// Returns whatever it managed to collect alongside any error: partial inventory
// still lets some checks produce findings, and a region that half-worked is more
// useful than a region reported as empty.
func fetchInventory(ctx context.Context, api EBSVolumesAPI, n needs, account, region string) (Inventory, error) {
	inv := Inventory{Account: account, Region: region}

	if n.volumes {
		vols, err := allVolumes(ctx, api)
		inv.Volumes = vols
		if err != nil {
			return inv, err
		}
	}

	return inv, nil
}

// allVolumes pages through every volume in the region. No filter: the planner
// fetches once and the checks narrow, so filtering here would defeat sharing.
func allVolumes(ctx context.Context, api EBSVolumesAPI) ([]ec2types.Volume, error) {
	var out []ec2types.Volume
	var token *string

	for {
		page, err := api.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: token})
		if err != nil {
			// Partial pages are kept: a truncated set the caller knows about
			// beats an empty set it does not.
			return out, fmt.Errorf("describe volumes: %w", err)
		}
		out = append(out, page.Volumes...)

		token = page.NextToken
		if token == nil {
			return out, nil
		}
	}
}
