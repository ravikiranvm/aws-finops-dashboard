package audit

// The v3.0 check set.
//
// Every check here is single-API arithmetic: no CloudWatch metrics, no
// conditional assumptions, no "if you would stop it" caveats. That is why all
// of them carry an exact basis — every dollar figure is defensible, and the
// fast tier is the only tier (OVERHAUL.md §C.0.2).
//
// Deferred to a later release: idle detection (needs CloudWatch), approximate
// checks (ELB with no targets, previous-generation families), and anything
// requiring a conditional basis.
//
// Descriptions are written for the catalogue's detail strip: what it looks for,
// and why it costs money. They are the answer to "what does this actually do".
func init() {
	// ---- Waste: money spent for nothing ----------------------------------

	Register(Check{
		ID:      "ebs-unattached",
		Name:    "Unattached EBS volumes",
		Service: "EBS",
		Description: "Volumes sitting in the 'available' state. They bill at the " +
			"full per-GB rate while attached to nothing at all.",
		Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs:           []string{"ec2:DescribeVolumes"},
		DefaultEnabled: true,
		Columns: []Column{
			{Header: "RESOURCE", Flex: true},
			{Key: "region", Header: "REGION", Width: 16, Shed: 2},
			{Key: "size_gb", Header: "SIZE", Width: 8, Right: true, Format: FormatGB},
			{Key: "volume_type", Header: "TYPE", Width: 6, Right: true, Shed: 1},
			{Key: "savings", Header: "$/MO", Width: 10, Right: true, Format: FormatMoney},
		},
	})

	Register(Check{
		ID:      "eip-unassociated",
		Name:    "Unassociated Elastic IPs",
		Service: "EC2",
		Description: "Elastic IPs allocated but not attached to a running instance. " +
			"AWS charges an hourly rate precisely because they are idle.",
		Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs:           []string{"ec2:DescribeAddresses"},
		DefaultEnabled: true,
	})

	Register(Check{
		ID:      "ec2-stopped-ebs",
		Name:    "Stopped EC2 with attached EBS",
		Service: "EC2",
		Description: "Stopped instances still pay for their attached volumes. The " +
			"compute is free; the storage is not, and it is easy to forget.",
		Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs:           []string{"ec2:DescribeInstances", "ec2:DescribeVolumes"},
		DefaultEnabled: true,
	})

	Register(Check{
		ID:      "ebs-orphaned-snapshots",
		Name:    "Orphaned EBS snapshots",
		Service: "EBS",
		Description: "Snapshots older than 90 days whose source volume no longer " +
			"exists and which back no AMI. They accumulate silently, forever.",
		Category: CategoryWaste, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs:           []string{"ec2:DescribeSnapshots", "ec2:DescribeImages"},
		DefaultEnabled: true,
	})

	// ---- Rate: paying more than necessary for the same thing -------------

	Register(Check{
		ID:      "ebs-gp2-to-gp3",
		Name:    "gp2 volumes that could be gp3",
		Service: "EBS",
		Description: "gp3 costs roughly 20% less than gp2 for the same capacity, " +
			"with baseline performance at least as good. The volume is doing real " +
			"work — you are simply overpaying for it.",
		Category: CategoryRate, Basis: BasisExact, Magnitude: MagnitudeSavings,
		APIs:           []string{"ec2:DescribeVolumes"},
		DefaultEnabled: true,
		Columns: []Column{
			{Header: "RESOURCE", Flex: true},
			{Key: "region", Header: "REGION", Width: 16, Shed: 2},
			{Key: "size_gb", Header: "SIZE", Width: 8, Right: true, Format: FormatGB},
			{Key: "volume_type", Header: "TYPE", Width: 6, Right: true, Shed: 1},
			{Key: "savings", Header: "$/MO", Width: 10, Right: true, Format: FormatMoney},
		},
	})

	// ---- Allocation: spend that cannot be attributed ---------------------

	Register(Check{
		ID:      "untagged-resources",
		Name:    "Untagged resources",
		Service: "All",
		Description: "Resources with no tags cannot be attributed to a team or " +
			"product, so their cost lands in an unallocated bucket nobody owns. " +
			"Found in one call per region via the Resource Groups Tagging API.",
		Category: CategoryAllocation, Basis: BasisNone, Magnitude: MagnitudeUnallocated,
		APIs:           []string{"tag:GetResources"},
		DefaultEnabled: true,
	})

	// ---- Financial risk --------------------------------------------------

	Register(Check{
		ID:      "budget-breached",
		Name:    "Budgets over their limit",
		Service: "Budgets",
		Description: "Budgets whose actual spend has already passed the configured " +
			"limit. Carried over from v2, where it was the only forward-looking signal.",
		Category: CategoryRisk, Basis: BasisNone, Magnitude: MagnitudeCount,
		CountLabel:     "breached",
		APIs:           []string{"budgets:DescribeBudgets"},
		DefaultEnabled: true,
		// Budgets live at the account level behind a single global endpoint, so
		// running this per region would report every budget once per region.
		Scope: ScopeAccount,
		// No savings column: a breached budget is a signal, not a number we can
		// offer to stop spending. OVER is the point of the check, so it never
		// sheds; FORECAST goes first because it is the one number that has not
		// happened yet.
		Columns: []Column{
			{Header: "BUDGET", Flex: true},
			{Key: "period", Header: "PERIOD", Width: 8, Shed: 2},
			{Key: "limit", Header: "LIMIT", Width: 12, Right: true, Format: FormatAmount},
			{Key: "actual", Header: "ACTUAL", Width: 12, Right: true, Format: FormatAmount},
			{Key: "forecast", Header: "FORECAST", Width: 12, Right: true, Format: FormatAmount, Shed: 1},
			{Key: "over", Header: "OVER", Width: 11, Right: true, Format: FormatAmount},
		},
	})
}
