package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Identity is who the resolved credentials belong to.
type Identity struct {
	ARN       string
	AccountID string
	UserID    string
	Region    string

	// Source records how the credentials were obtained, so the header strip
	// can say "SSO acme-sso" rather than just showing an account number.
	Source     Kind
	SourceName string
	Expiry     time.Time
}

// DisplayName is the short label for the header strip: the role or user name
// rather than the full ARN, which never fits.
func (i Identity) DisplayName() string {
	// arn:aws:sts::123456789012:assumed-role/FinOpsReadOnly/ravikiran
	if idx := strings.LastIndex(i.ARN, "/"); idx >= 0 && idx < len(i.ARN)-1 {
		return i.ARN[idx+1:]
	}
	if i.AccountID != "" {
		return i.AccountID
	}
	return "unknown"
}

// Account is one AWS account we can see. In org mode these come from
// Organizations; otherwise there is exactly one, our own.
type Account struct {
	ID     string
	Name   string
	Email  string
	Status string
}

// Org describes the Organizations context, if any.
type Org struct {
	// IsManagement is true when our credentials belong to the management
	// (payer) account — the case where payer-level Cost Explorer works and
	// one API call covers every member account.
	IsManagement bool
	ID           string
	MasterID     string
	Accounts     []Account
}

// Resolver turns ambient configuration into a usable identity.
type Resolver struct {
	env Env
}

func NewResolver(env Env) *Resolver { return &Resolver{env: env} }

// Options select a specific credential source. Zero value means "use the
// default chain": env → SSO → shared profile → IRSA/ECS/IMDS → OIDC.
type Options struct {
	Profile string
	Region  string
}

// Resolve loads credentials and confirms who they belong to.
//
// It always calls sts:GetCallerIdentity — an unverified identity is worse than
// none, because every later screen would attribute data to the wrong account.
func (r *Resolver) Resolve(ctx context.Context, opts Options) (Identity, aws.Config, error) {
	d := Discover(r.env)

	// Nothing is configured, so there is nothing to resolve. Bailing out here
	// is not just tidy: letting the SDK run its full chain costs ~5 seconds,
	// almost all of it retrying EC2 instance metadata that cannot exist on a
	// laptop. Offline discovery already knows the answer.
	if opts.Profile == "" && len(d.Available()) == 0 {
		return Identity{}, aws.Config{}, errNoCredentials()
	}

	var loadOpts []func(*awsconfig.LoadOptions) error

	// Only reach for instance metadata when something suggests we are on an
	// instance. Off-instance the probe is pure latency.
	if !d.hasAmbient() {
		loadOpts = append(loadOpts,
			awsconfig.WithEC2IMDSClientEnableState(imds.ClientDisabled))
	}

	if opts.Profile != "" {
		// Check the profile exists before the SDK reports it in its own words.
		// "failed to get shared config profile, x" gives the user nothing to
		// act on; naming the profiles that *do* exist does.
		if !contains(d.Profiles, opts.Profile) {
			return Identity{}, aws.Config{}, errUnknownProfile(opts.Profile, d.Profiles)
		}
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(opts.Profile))
	}
	if opts.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(opts.Region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return Identity{}, cfg, errAWS("loading AWS configuration", err)
	}

	// Credentials are lazy in the SDK, so this is the first point anything can
	// actually fail. Translate SDK errors into road signs before they escape.
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return Identity{}, cfg, r.explainCredentialFailure(err)
	}

	id := Identity{
		ARN:       aws.ToString(out.Arn),
		AccountID: aws.ToString(out.Account),
		UserID:    aws.ToString(out.UserId),
		Region:    cfg.Region,
	}
	r.annotateSource(&id, opts, d)
	return id, cfg, nil
}

// annotateSource works out which discovered source produced these credentials,
// so the UI can name it. This is best-effort labelling: the credentials are
// already valid at this point, so a wrong guess costs a cosmetic detail rather
// than correctness.
func (r *Resolver) annotateSource(id *Identity, opts Options, d Discovery) {

	if opts.Profile != "" {
		id.Source, id.SourceName = KindProfile, opts.Profile
		return
	}
	if r.env.Get("AWS_ACCESS_KEY_ID") != "" {
		id.Source = KindEnv
		return
	}
	if r.env.Get("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
		id.Source = KindOIDC
		return
	}
	if p := r.env.Get("AWS_PROFILE"); p != "" {
		id.Source, id.SourceName = KindProfile, p
		return
	}
	// An unexpired SSO session is the most likely remaining explanation.
	for _, s := range d.Sources {
		if s.Kind == KindSSO && s.Available && !s.Expired() {
			id.Source, id.SourceName, id.Expiry = KindSSO, s.Name, s.Expiry
			return
		}
	}
	for _, s := range d.Sources {
		if s.Kind == KindAmbient && s.Available {
			id.Source = KindAmbient
			return
		}
	}
	// Falling through to the shared config's default profile is the common
	// laptop case; naming it beats saying "default credential chain", which
	// tells the user nothing they can act on.
	for _, p := range d.Profiles {
		if p == "default" {
			id.Source, id.SourceName = KindProfile, "default"
			return
		}
	}
}

// explainCredentialFailure converts an SDK error into something actionable.
// The SDK's own messages name internal providers and are not useful to a user
// who just needs to know which command to run.
func (r *Resolver) explainCredentialFailure(err error) error {
	d := Discover(r.env)

	// An expired SSO session is the single most common failure, and the one
	// with the most specific fix.
	for _, s := range d.Sources {
		if s.Kind == KindSSO && s.Expired() {
			return errAuthExpired(s.Name, humanDuration(time.Since(s.Expiry)))
		}
	}

	var oe interface{ ErrorCode() string }
	if errors.As(err, &oe) {
		switch oe.ErrorCode() {
		case "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId":
			return &Error{
				Code:        CodeAuthExpired,
				Message:     "AWS credentials have expired",
				Remediation: "refresh them — 'aws sso login', or re-issue your session token",
				Exit:        3,
				Err:         err,
			}
		case "AccessDenied", "AccessDeniedException":
			return &Error{
				Code:        CodePermission,
				Message:     "credentials resolved but sts:GetCallerIdentity was denied",
				Remediation: "grant sts:GetCallerIdentity — it is required to identify the account",
				Exit:        4,
				Err:         err,
			}
		}
	}

	if len(d.Available()) == 0 {
		return errNoCredentials()
	}

	// Sources are configured but none currently hold usable credentials — an
	// SSO session that was never signed into, or profiles that depend on one.
	// This is the "pick something and sign in" case, not a broken tool, so it
	// must carry CodeNoCredentials for the picker to open. It also must not
	// surface the SDK's own message, which at this point is a chain of
	// provider internals ending in "no EC2 IMDS role found" — true, useless,
	// and alarming.
	if names := signInCandidates(d); len(names) > 0 {
		return &Error{
			Code:        CodeNoCredentials,
			Message:     "credentials are configured but you are not signed in",
			Remediation: fmt.Sprintf("aws sso login --sso-session %s", names[0]),
			Exit:        3,
			Err:         err,
		}
	}
	return errAWS("verifying identity", err)
}

// signInCandidates lists SSO sessions that exist but hold no valid token.
func signInCandidates(d Discovery) []string {
	var out []string
	for _, s := range d.Sources {
		if s.Kind == KindSSO && s.Available && (s.Expired() || s.Note == "not signed in") {
			out = append(out, s.Name)
		}
	}
	return out
}

// DescribeOrg reports the Organizations context for the resolved identity.
//
// Not being in an organization, and not being allowed to ask, are both normal:
// they mean "single account mode", not failure. Only the management account
// can enumerate members, which is exactly the case we care about.
func DescribeOrg(ctx context.Context, cfg aws.Config, id Identity) (Org, error) {
	client := organizations.NewFromConfig(cfg)

	desc, err := client.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		// AWSOrganizationsNotInUseException / AccessDenied both mean "treat
		// this as a single account", not "stop".
		return Org{}, nil //nolint:nilerr // absence of an org is not an error
	}

	org := Org{
		ID:       aws.ToString(desc.Organization.Id),
		MasterID: aws.ToString(desc.Organization.MasterAccountId),
	}
	org.IsManagement = org.MasterID == id.AccountID
	if !org.IsManagement {
		// A member account can see the org exists but cannot list its peers.
		return org, nil
	}

	paginator := organizations.NewListAccountsPaginator(client, &organizations.ListAccountsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			// Partial results beat none: report what we enumerated and let
			// the caller show the org as partially discovered.
			return org, errAWS("listing organization accounts", err)
		}
		for _, a := range page.Accounts {
			org.Accounts = append(org.Accounts, Account{
				ID:     aws.ToString(a.Id),
				Name:   aws.ToString(a.Name),
				Email:  aws.ToString(a.Email),
				Status: string(a.Status),
			})
		}
	}
	return org, nil
}

// Summary is the one-line identity strip for the top bar. It abbreviates
// deliberately: the full identity runs to ~66 characters and cannot share a
// line with a breadcrumb at 80 columns (OVERHAUL.md D-B4).
func (i Identity) Summary(org Org) string {
	var b strings.Builder
	b.WriteString(i.DisplayName())

	if org.IsManagement && len(org.Accounts) > 0 {
		fmt.Fprintf(&b, " · org %d", len(org.Accounts))
	} else {
		b.WriteString(" · " + i.AccountID)
	}

	// Expiry is shown only when it is nearly out — otherwise it is noise.
	if !i.Expiry.IsZero() {
		if left := time.Until(i.Expiry); left < time.Hour {
			fmt.Fprintf(&b, " · SSO %s %s", humanDuration(left), "⚠")
		}
	}
	return b.String()
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
