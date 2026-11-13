package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// Pricer supplies on-demand prices so findings can carry defensible dollar
// figures. Everything it returns is exact arithmetic from published prices —
// no estimation, no heuristics.
type Pricer interface {
	// EBSPerGBMonth is the monthly price of one GB of a given volume type in a
	// given region. Returns ErrNoPrice when the price is unavailable, which
	// callers must treat as "report the finding without money" rather than as
	// a failure.
	EBSPerGBMonth(ctx context.Context, region, volumeType string) (float64, error)
}

// ErrNoPrice means a price could not be determined. A denied pricing:GetProducts
// or an unrecognised volume type both land here. It is not a scan failure: eight
// unattached volumes are worth reporting even without a dollar figure.
var ErrNoPrice = fmt.Errorf("no price available")

// AWSPricer reads the AWS Price List API and caches per process.
//
// Prices change rarely, so a lookup is amortised across every finding in a run
// and every region shares the same client. The Price List API is only served
// from a few endpoints; us-east-1 is always one.
type AWSPricer struct {
	client *pricing.Client

	mu    sync.Mutex
	cache map[string]float64 // "region/volumeType" → USD per GB-month
}

func NewAWSPricer(cfg aws.Config) *AWSPricer {
	return &AWSPricer{
		client: pricing.NewFromConfig(cfg, func(o *pricing.Options) { o.Region = "us-east-1" }),
		cache:  map[string]float64{},
	}
}

func (p *AWSPricer) EBSPerGBMonth(ctx context.Context, region, volumeType string) (float64, error) {
	key := region + "/" + volumeType

	p.mu.Lock()
	if v, ok := p.cache[key]; ok {
		p.mu.Unlock()
		return v, nil
	}
	p.mu.Unlock()

	price, err := p.fetchEBS(ctx, region, volumeType)
	if err != nil {
		return 0, err
	}

	p.mu.Lock()
	p.cache[key] = price
	p.mu.Unlock()
	return price, nil
}

func (p *AWSPricer) fetchEBS(ctx context.Context, region, volumeType string) (float64, error) {
	out, err := p.client.GetProducts(ctx, &pricing.GetProductsInput{
		ServiceCode: aws.String("AmazonEC2"),
		MaxResults:  aws.Int32(10),
		Filters: []types.Filter{
			// regionCode avoids mapping region codes to the API's display
			// names ("Asia Pacific (Singapore)"), which are a moving target.
			{Type: types.FilterTypeTermMatch, Field: aws.String("regionCode"), Value: aws.String(region)},
			{Type: types.FilterTypeTermMatch, Field: aws.String("productFamily"), Value: aws.String("Storage")},
			{Type: types.FilterTypeTermMatch, Field: aws.String("volumeApiName"), Value: aws.String(volumeType)},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrNoPrice, err)
	}
	for _, raw := range out.PriceList {
		if v, ok := firstOnDemandUSD(raw); ok {
			return v, nil
		}
	}
	return 0, ErrNoPrice
}

// firstOnDemandUSD digs the USD unit price out of a Price List entry.
//
// The shape is genuinely this deep:
//
//	terms.OnDemand.<offerCode>.priceDimensions.<rateCode>.pricePerUnit.USD
//
// There is no typed accessor — GetProducts returns opaque JSON strings — so the
// traversal is hand-rolled and defensive at every level.
func firstOnDemandUSD(raw string) (float64, bool) {
	var doc struct {
		Terms struct {
			OnDemand map[string]struct {
				PriceDimensions map[string]struct {
					PricePerUnit map[string]string `json:"pricePerUnit"`
					Unit         string            `json:"unit"`
				} `json:"priceDimensions"`
			} `json:"OnDemand"`
		} `json:"terms"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return 0, false
	}

	for _, offer := range doc.Terms.OnDemand {
		for _, dim := range offer.PriceDimensions {
			usd, ok := dim.PricePerUnit["USD"]
			if !ok {
				continue
			}
			v, err := strconv.ParseFloat(usd, 64)
			if err != nil || v <= 0 {
				continue
			}
			return v, true
		}
	}
	return 0, false
}

// StaticPricer returns fixed prices. Used in tests so evaluator logic can be
// verified without network calls or a live account.
type StaticPricer struct {
	PerGBMonth map[string]float64 // volume type → price
}

func (s StaticPricer) EBSPerGBMonth(_ context.Context, _ string, volumeType string) (float64, error) {
	if v, ok := s.PerGBMonth[volumeType]; ok {
		return v, nil
	}
	return 0, ErrNoPrice
}
