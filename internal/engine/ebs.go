package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/ravikiranvm/aws-finops-dashboard/internal/audit"
)

// EBSVolumesAPI is the slice of EC2 this evaluator needs. Narrowing it to one
// method is what lets the logic be tested with a hand-written fake instead of a
// container or a live account.
type EBSVolumesAPI interface {
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}

// UnattachedEBS finds volumes in the "available" state — attached to nothing,
// billing at the full per-GB rate.
//
// Savings are exact: size × published price for that volume type and region.
// No assumption about what the user would do, which is why the basis is exact
// rather than conditional.
type UnattachedEBS struct{}

func (UnattachedEBS) CheckID() string { return "ebs-unattached" }

func (e UnattachedEBS) Evaluate(ctx context.Context, inv Inventory, pricer Pricer) []audit.Finding {
	var findings []audit.Finding
	for _, v := range inv.Volumes {
		if v.State != ec2types.VolumeStateAvailable {
			continue
		}
		findings = append(findings, e.finding(ctx, pricer, inv.Account, inv.Region, v))
	}
	return findings
}

func (e UnattachedEBS) finding(
	ctx context.Context,
	pricer Pricer,
	account, region string,
	v ec2types.Volume,
) audit.Finding {
	id := aws.ToString(v.VolumeId)
	sizeGB := int(aws.ToInt32(v.Size))
	volType := string(v.VolumeType)

	f := audit.Finding{
		Fingerprint: audit.NewFingerprint(e.CheckID(), account, region, id),
		CheckID:     e.CheckID(),
		Service:     "EBS",
		Category:    audit.CategoryWaste,
		Account:     account,
		Region:      region,
		ResourceID:  id,
		Title:       fmt.Sprintf("%s · %d GB %s · unattached", id, sizeGB, volType),
		Owner:       ownerFromTags(v.Tags),
		Detail: map[string]any{
			"size_gb":     sizeGB,
			"volume_type": volType,
			"encrypted":   aws.ToBool(v.Encrypted),
			"created":     aws.ToTime(v.CreateTime).Format(time.RFC3339),
			"age_days":    int(time.Since(aws.ToTime(v.CreateTime)).Hours() / 24),
		},
		Magnitude: audit.Magnitude{
			Kind:  audit.MagnitudeSavings,
			Basis: audit.BasisExact,
		},
		FirstSeen: time.Now(),
	}

	perGB, err := pricer.EBSPerGBMonth(ctx, region, volType)
	if err != nil {
		// Pricing unavailable — usually pricing:GetProducts denied. Report the
		// volume anyway: eight unattached volumes are worth knowing about even
		// without the money. Silently showing $0 would be a lie.
		if errors.Is(err, ErrNoPrice) {
			f.Evidence = []string{fmt.Sprintf("%d GB %s · price unavailable", sizeGB, volType)}
			return f
		}
		f.Evidence = []string{fmt.Sprintf("%d GB %s · pricing lookup failed", sizeGB, volType)}
		return f
	}

	monthly := float64(sizeGB) * perGB
	f.Magnitude.USDPerMonth = monthly
	// Evidence is the arithmetic, shown in the resource view so anyone can
	// check the number rather than trusting it.
	f.Evidence = []string{
		fmt.Sprintf("%d GB × $%.4f/GB-month (%s, %s)", sizeGB, perGB, volType, region),
		fmt.Sprintf("= $%.2f per month", monthly),
	}
	return f
}

// ownerFromTags derives an owner from the tags teams actually use. A finding
// nobody owns cannot be actioned, so the absence is reported rather than hidden.
func ownerFromTags(tags []ec2types.Tag) string {
	for _, want := range []string{"Owner", "owner", "Team", "team", "CostCenter", "cost-center"} {
		for _, t := range tags {
			if aws.ToString(t.Key) == want {
				if v := aws.ToString(t.Value); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// GP2ToGP3 finds gp2 volumes that would cost less as gp3 at the same size.
//
// This is Rate, not Waste: the volume is doing real work. You are simply paying
// more than necessary for it. gp3 lists ~20% below gp2 everywhere, with baseline
// performance at least as good, so the saving is the price delta — exact
// arithmetic, no assumption about what the user would do.
//
// It reads the same DescribeVolumes call as UnattachedEBS, which is why the
// planner deduplicates API calls across checks rather than letting each fetch
// its own copy.
type GP2ToGP3 struct{}

func (GP2ToGP3) CheckID() string { return "ebs-gp2-to-gp3" }

func (e GP2ToGP3) Evaluate(ctx context.Context, inv Inventory, pricer Pricer) []audit.Finding {
	var findings []audit.Finding
	for _, v := range inv.Volumes {
		if v.VolumeType != ec2types.VolumeTypeGp2 {
			continue
		}
		findings = append(findings, e.finding(ctx, pricer, inv.Account, inv.Region, v))
	}
	return findings
}

func (e GP2ToGP3) finding(
	ctx context.Context,
	pricer Pricer,
	account, region string,
	v ec2types.Volume,
) audit.Finding {
	id := aws.ToString(v.VolumeId)
	sizeGB := int(aws.ToInt32(v.Size))

	f := audit.Finding{
		Fingerprint: audit.NewFingerprint(e.CheckID(), account, region, id),
		CheckID:     e.CheckID(),
		Service:     "EBS",
		Category:    audit.CategoryRate,
		Account:     account,
		Region:      region,
		ResourceID:  id,
		Title:       fmt.Sprintf("%s · %d GB gp2 → gp3", id, sizeGB),
		Owner:       ownerFromTags(v.Tags),
		Detail: map[string]any{
			"size_gb":     sizeGB,
			"volume_type": "gp2",
			"attached":    len(v.Attachments) > 0,
		},
		Magnitude: audit.Magnitude{Kind: audit.MagnitudeSavings, Basis: audit.BasisExact},
		FirstSeen: time.Now(),
	}

	gp2, err2 := pricer.EBSPerGBMonth(ctx, region, "gp2")
	gp3, err3 := pricer.EBSPerGBMonth(ctx, region, "gp3")
	if err2 != nil || err3 != nil {
		f.Evidence = []string{fmt.Sprintf("%d GB gp2 · price unavailable", sizeGB)}
		return f
	}

	delta := gp2 - gp3
	if delta <= 0 {
		// gp3 is not cheaper here. Not a finding — reporting it would be noise.
		f.Evidence = []string{fmt.Sprintf("gp3 is not cheaper in %s", region)}
		return f
	}

	monthly := float64(sizeGB) * delta
	f.Magnitude.USDPerMonth = monthly
	f.Evidence = []string{
		fmt.Sprintf("gp2 $%.4f − gp3 $%.4f = $%.4f per GB-month (%s)", gp2, gp3, delta, region),
		fmt.Sprintf("%d GB × $%.4f = $%.2f per month", sizeGB, delta, monthly),
	}
	return f
}
