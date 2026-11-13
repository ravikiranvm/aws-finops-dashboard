package engine

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
	"github.com/ravikiranvm/aws-finops-dashboard/internal/core"
)

// Target is one account and the credentials to reach it.
//
// A list rather than a single config from day one: org mode means a different
// assumed-role config per member account, and retrofitting that through every
// evaluator later would be painful.
type Target struct {
	Account core.Account
	Config  aws.Config
}

// Request is everything a scan needs. See OVERHAUL.md for the reasoning behind
// each field being explicit rather than implied.
type Request struct {
	Targets   []Target
	Regions   []string
	Checks    []audit.Check
	Preflight core.Preflight
	Pricer    Pricer
	// Concurrency bounds simultaneous region×check work. Zero means a default.
	Concurrency int
}

// Result is what a scan produced, and honestly what it did not.
type Result struct {
	Findings []audit.Finding
	Coverage audit.Coverage
	Failures []audit.Failure
	Duration time.Duration
}

// TotalSavings sums only priced savings findings. Deliberately narrow: summing
// across magnitude kinds would add savings to unattributed spend, which is
// meaningless, and counting unpriced findings as zero would understate silently.
func (r Result) TotalSavings() (usd float64, priced, unpriced int) {
	for _, f := range r.Findings {
		if f.Magnitude.Kind != audit.MagnitudeSavings {
			continue
		}
		if f.Magnitude.Priced() {
			usd += f.Magnitude.USDPerMonth
			priced++
		} else {
			unpriced++
		}
	}
	return usd, priced, unpriced
}

// evaluator is one runnable check. Registered here rather than in the audit
// package so the declarations stay free of AWS dependencies.
type evaluator interface {
	CheckID() string
	// Evaluate is a pure function over the shared inventory: no API calls, no
	// pagination, no error path. Fetching is the planner's job, which is what
	// makes checks trivially testable and API calls shareable.
	Evaluate(ctx context.Context, inv Inventory, pricer Pricer) []audit.Finding
}

// evaluators maps declared checks to their implementations. A check declared in
// the registry with no evaluator here is reported as skipped rather than
// silently contributing nothing — an unimplemented check and a clean result
// must not look the same.
var evaluators = map[string]evaluator{
	"ebs-unattached":  UnattachedEBS{},
	"ebs-gp2-to-gp3":  GP2ToGP3{},
	"budget-breached": BudgetsBreached{},
}

// Implemented reports whether a declared check can actually run yet.
func Implemented(checkID string) bool {
	_, ok := evaluators[checkID]
	return ok
}

// Run executes the requested checks across every target and region.
//
// Failures degrade in place: one inaccessible region is recorded and excluded,
// never fatal (OVERHAUL.md R-B14). Coverage records what was genuinely scanned,
// so a total can never imply completeness it does not have.
func Run(ctx context.Context, req Request) Result {
	start := time.Now()

	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 8
	}

	result := Result{
		Coverage: audit.Coverage{
			AccountsRequested: len(req.Targets),
			RegionsRequested:  len(req.Regions),
			ChecksRequested:   len(req.Checks),
			ChecksSkipped:     map[string]string{},
		},
	}

	// Decide up front which checks can run at all, so skips are reported once
	// rather than per region.
	var runnable []audit.Check
	for _, c := range req.Checks {
		switch {
		case !Implemented(c.ID):
			result.Coverage.ChecksSkipped[c.ID] = "not implemented yet"
		case !permitted(req.Preflight, c):
			result.Coverage.ChecksSkipped[c.ID] = "missing permission: " + missingAPI(req.Preflight, c)
		default:
			runnable = append(runnable, c)
		}
	}
	result.Coverage.ChecksRun = len(runnable)

	// Split by fan-out. An account-scoped check sees the same answer from every
	// region, so running it in the regional loop would multiply every finding by
	// the number of regions scanned.
	var perRegion, accountWide []audit.Check
	for _, c := range runnable {
		if c.Fanout() == audit.ScopeAccount {
			accountWide = append(accountWide, c)
			continue
		}
		perRegion = append(perRegion, c)
	}

	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		sem      = make(chan struct{}, concurrency)
		scanned  = map[string]bool{} // region -> reached at least once
		accounts = map[string]bool{}
	)

	// Account-scoped work: one unit per account, whatever the region list says.
	for _, target := range req.Targets {
		if len(accountWide) == 0 {
			break
		}
		wg.Add(1)
		go func(target Target) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Budgets and the other account-wide APIs are global endpoints. The
			// SDK still wants a region on the config; us-east-1 is where they
			// are homed.
			global := target.Config.Copy()
			global.Region = "us-east-1"

			var apis []string
			for _, c := range accountWide {
				apis = append(apis, c.APIs...)
			}

			inv, err := fetchAccountInventory(ctx, budgets.NewFromConfig(global),
				planNeeds(apis), target.Account.ID)

			mu.Lock()
			if err != nil {
				result.Failures = append(result.Failures, audit.Failure{
					Account: target.Account.ID, Region: "global",
					CheckID: "inventory", Message: err.Error(),
				})
				mu.Unlock()
				return
			}
			accounts[target.Account.ID] = true
			mu.Unlock()

			for _, check := range accountWide {
				found := evaluators[check.ID].Evaluate(ctx, inv, req.Pricer)
				mu.Lock()
				result.Findings = append(result.Findings, found...)
				mu.Unlock()
			}
		}(target)
	}

	// One unit of work per account+region, not per account+region+check: the
	// inventory is fetched once and every check reads the same copy.
	for _, target := range req.Targets {
		for _, region := range req.Regions {
			if len(perRegion) == 0 {
				break
			}
			wg.Add(1)
			go func(target Target, region string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				regional := target.Config.Copy()
				regional.Region = region
				client := ec2.NewFromConfig(regional)

				var apis []string
				for _, c := range perRegion {
					apis = append(apis, c.APIs...)
				}

				inv, err := fetchInventory(ctx, client, planNeeds(apis), target.Account.ID, region)

				mu.Lock()
				if err != nil {
					result.Failures = append(result.Failures, audit.Failure{
						Account: target.Account.ID, Region: region,
						CheckID: "inventory", Message: err.Error(),
					})
					mu.Unlock()
					return
				}
				scanned[region] = true
				accounts[target.Account.ID] = true
				mu.Unlock()

				for _, check := range perRegion {
					found := evaluators[check.ID].Evaluate(ctx, inv, req.Pricer)
					mu.Lock()
					result.Findings = append(result.Findings, found...)
					mu.Unlock()
				}
			}(target, region)
		}
	}
	wg.Wait()

	result.Coverage.RegionsScanned = len(scanned)
	result.Coverage.AccountsScanned = len(accounts)
	result.Duration = time.Since(start)
	return result
}

// permitted reports whether the preflight saw every API a check needs. An
// unchecked permission counts as permitted: we did not probe Cost Explorer
// because probing is billable, and refusing to run on that basis would be worse.
func permitted(p core.Preflight, c audit.Check) bool {
	if len(p.Permissions) == 0 {
		return true // no preflight ran; let the call fail with a real error
	}
	for _, api := range c.APIs {
		if state, known := stateOf(p, api); known && state == core.PermDenied {
			return false
		}
	}
	return true
}

func missingAPI(p core.Preflight, c audit.Check) string {
	for _, api := range c.APIs {
		if state, known := stateOf(p, api); known && state == core.PermDenied {
			return api
		}
	}
	return "unknown"
}

func stateOf(p core.Preflight, action string) (core.PermissionState, bool) {
	for _, perm := range p.Permissions {
		if perm.Action == action {
			return perm.State, true
		}
	}
	return "", false
}
