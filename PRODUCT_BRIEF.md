# Product Brief — AWS FinOps Dashboard (CLI)

**Version:** 2.3.0 · **License:** MIT · **Package:** [`aws-finops-dashboard`](https://pypi.org/project/aws-finops-dashboard/) · **Command:** `aws-finops`

---

## 1. What it is

AWS FinOps Dashboard is an open-source, terminal-based cost and resource dashboard for AWS. It reads your existing AWS CLI profiles and renders a live, colour-formatted view of spend, budgets, EC2 footprint, six-month cost trends, and resource-hygiene findings — then exports any of it to CSV, JSON, or PDF, locally or straight to S3 or Slack.

No agent to deploy, no infrastructure to provision, no account to create. One `pipx install`, and it runs against the credentials already on your machine.

## 2. Who it's for

| Audience | What they get |
|---|---|
| **Cloud / DevOps engineers** | A fast spend check without opening the AWS Console or clicking through Cost Explorer. |
| **FinOps practitioners & analysts** | Multi-account cost breakdowns by service and cost allocation tag, exportable for reporting. |
| **Platform / SRE teams** | Recurring hygiene audits — untagged, stopped, and orphaned resources across regions. |
| **Engineering managers & finance** | Scheduled PDF/CSV reports delivered to Slack or S3, no CLI access needed to consume them. |
| **MSPs and consultancies** | One command sweeps every client profile in the AWS config and combines by account. |

## 3. The problem

AWS cost data is spread across Cost Explorer, Budgets, and per-service consoles, and none of it is multi-account by default. Answering "what did we spend last month, per account, per service, and is anything untagged or idle?" means a lot of tab-switching, or building your own tooling on the Cost Explorer API. Existing FinOps platforms solve this, but require onboarding, access grants, and often a contract.

## 4. What it does — full feature inventory

### 4.1 Cost analysis

- **Default period view** — current month and previous month spend, side by side, for every profile queried.
- **Custom time ranges** — `--time-range 7 | 30 | 90` (any integer number of days).
- **Previous calendar month** — `--time-range last-month` queries the full prior month, including its per-service breakdown.
- **Cost by AWS service** — per-service spend for both periods, sorted highest-cost first.
- **Cost by cost allocation tag** — `--tag Team=DevOps Env=Prod` filters cost data to one or more `Key=Value` tags (requires cost allocation tags to be activated in the payer account).
- **AWS Budgets** — each budget's limit alongside actual spend, so overruns are visible in the same table as the costs driving them.
- **EC2 instance status** — instance counts by state (running, stopped, etc.) across the regions checked.

### 4.2 Cost trend analysis (`--trend`)

- Six-month spend history per profile, rendered as bar charts directly in the terminal.
- Works with `--tag`, so you can trend a single team, project, or cost centre.
- Composable with `--all` to trend every profile in one run.

### 4.3 FinOps audit (`--audit`)

A resource-hygiene sweep that costs nothing in API charges. Covers:

- **Untagged resources** — across **EC2 instances, RDS DB instances, Lambda functions, and ELBv2 load balancers**, reported per region.
- **Stopped EC2 instances** — still incurring EBS cost while doing no work.
- **Unused EBS volumes** — unattached volumes billing quietly.
- **Unused Elastic IPs** — unassociated EIPs, which AWS charges for.
- **Budget breaches / alerts** — budgets already exceeded or alerting.

### 4.4 Profile and region control

- **Automatic profile detection** — reads named profiles from your AWS config; uses `default` if present.
- **Explicit selection** — `--profiles dev prod` (space-separated).
- **Everything** — `--all` runs against every profile found.
- **Deduplication** — `--combine` merges profiles that resolve to the same AWS account into single rows.
- **Region scoping** — `--regions us-east-1 eu-west-1 ap-southeast-2` for EC2/resource discovery; falls back to all accessible regions when omitted.

### 4.5 Export and delivery

| Destination | How | Notes |
|---|---|---|
| **CSV** | `--report-type csv` | Multi-line cells for per-service, budget, and EC2 detail — open in a spreadsheet, not a text editor. |
| **JSON** | `--report-type json` | Structured output for programmatic consumption. |
| **PDF** | `--report-type pdf` | Formatted report for both cost dashboards and audits. |
| **Multiple at once** | `--report-type csv json pdf` | All three in a single run. |
| **Local directory** | `--dir ./reports` | Defaults to the current working directory. |
| **Amazon S3** | `--s3-bucket my-bucket --s3-profile prod [--s3-prefix reports/2026]` | Uploads instead of saving locally; `--s3-profile` is mandatory. |
| **Slack** | `--slack C1234567890` | Posts the report files to a channel; requires the `SLACK_BOT_TOKEN` env var. |

**Cost dashboard PDF** contains: profile and account ID, previous and current period costs, per-service breakdown, budget status, EC2 summary.
**Audit PDF** contains: profile and account ID, untagged resources, stopped EC2 instances, unused volumes, unused EIPs, budget alerts.

**Known limitation:** trend reports (`--trend`) export to **JSON only** — other formats passed to `--report-type` are ignored for trend runs.

### 4.6 Configuration files

Drive every option from a file instead of flags, via `--config-file` / `-C`, in **TOML, YAML, or JSON**. Supported keys: `profiles`, `regions`, `combine`, `report_name`, `report_type`, `dir`, `time_range`, `tag`, `audit`, `trend`, `s3_bucket`, `s3_prefix`, `s3_profile`, `slack`.

Command-line arguments always override file settings, so one committed config can be shared by a team and overridden per-run.

### 4.7 Experience and operational details

- **Rich terminal UI** — live-updating, colour-coded tables and an ASCII welcome banner.
- **Automatic version check** — pings PyPI on startup and tells you if a newer release exists (silent on failure; 3-second timeout).
- **Resilient error handling** — a region or service the profile can't reach produces a warning and the run continues, rather than aborting.
- **Secure-by-default Slack token** — `SLACK_BOT_TOKEN` is env-var only, deliberately not accepted via config file or CLI flag.
- **Exit codes** — `0` on success, `1` on failure, so it drops into CI/cron cleanly.

## 5. Requirements to use it

### 5.1 Runtime

- **Python 3.8 or later** (declared `requires-python = ">=3.8"`).
- **A configured AWS CLI profile** — `aws configure --profile <name>`, repeated for each account you want visible.
- Terminal with ANSI colour support for the best rendering.
- Runtime dependencies, installed automatically: `boto3`, `rich`, `requests`, `packaging`, `reportlab`, `pyyaml`, `tomli`, `slack-sdk`.

### 5.2 IAM permissions

The credentials behind each profile need:

**Core (cost dashboard)**
`ce:GetCostAndUsage` · `budgets:ViewBudget` · `sts:GetCallerIdentity` · `ec2:DescribeInstances` · `ec2:DescribeRegions`

**Audit (`--audit`)**
`ec2:DescribeVolumes` · `ec2:DescribeAddresses` · `rds:DescribeDBInstances` · `rds:ListTagsForResource` · `lambda:ListFunctions` · `lambda:ListTags` · `elbv2:DescribeLoadBalancers` · `elbv2:DescribeTags`

**S3 export (`--s3-bucket`)**
`s3:PutObject` · `s3:ListBucket`

All are read-only apart from `s3:PutObject`. The tool never modifies or deletes AWS resources.

### 5.3 Optional — Slack delivery

1. Create a Slack app at api.slack.com/apps.
2. Grant bot token scopes `files:write` and `chat:write`.
3. Install to the workspace and copy the `xoxb-` Bot User OAuth Token.
4. `export SLACK_BOT_TOKEN=xoxb-...`
5. Invite the bot to the target channel — otherwise you'll get `not_in_channel` or `channel_not_found`.

### 5.4 Optional — cost allocation tags

`--tag` filtering only returns data for tags **activated as cost allocation tags** in the AWS Billing console. Un-activated tags return nothing.

## 6. Installation options

| Method | Command |
|---|---|
| **pipx** (recommended) | `pipx install aws-finops-dashboard` |
| **pip** | `pip install aws-finops-dashboard` |
| **uv** | `uv venv && source .venv/bin/activate && uv pip install aws-finops-dashboard` |
| **From source** | `git clone …/aws-finops-dashboard && pip install -e .` |
| **Docker** | `docker compose build` then `docker compose run --rm aws-finops --all` — mounts `~/.aws` automatically |

## 7. Cost of running it

The tool itself is free; AWS charges for Cost Explorer API calls (typically **$0.01 per call**).

| Mode | Approximate cost |
|---|---|
| Default cost dashboard | **$0.06** for the first profile, **+$0.03** per additional profile |
| Trend dashboard (`--trend`) | **$0.03** per profile |
| Audit (`--audit`) | **Free** — no Cost Explorer calls |

Keep it down by naming only the profiles you need with `--profiles`, and using `--combine` when several profiles point at one account.

## 8. Differentiators

1. **Zero onboarding** — no SaaS signup, no cross-account role to provision, no data leaving your machine unless you export it.
2. **Terminal-native** — lives where engineers already work; no browser round-trip for a spend check.
3. **Multi-account by default** — `--all --combine` is a first-class path, not an enterprise upsell.
4. **Cost *and* hygiene in one tool** — spend numbers plus the untagged and idle resources behind them.
5. **Automation-ready** — config files, exit codes, S3 and Slack delivery make it a cron job as easily as an interactive command.
6. **Audit mode is free** — the highest-value recurring check costs nothing to run daily.
7. **MIT-licensed and open source** — auditable, forkable, no vendor lock-in.

## 9. Try it in under a minute

```bash
pipx install aws-finops-dashboard

aws-finops --region us-east-1                    # cost dashboard, default profile
aws-finops --profiles dev prod --region us-east-1 # specific profiles
aws-finops --trend                               # 6-month trend bars
aws-finops --audit --region us-east-1            # free hygiene audit
aws-finops --report-name my_report --report-type pdf  # export to PDF
```

## 10. Links

- **Source:** https://github.com/ravikiranvm/aws-finops-dashboard
- **PyPI:** https://pypi.org/project/aws-finops-dashboard/
- **Author:** Ravi Kiran Vallamkonda
