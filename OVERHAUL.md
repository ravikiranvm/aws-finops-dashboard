# AWS FinOps Dashboard — v3 Overhaul: Requirements & Decision Log

**Status:** Discussion in progress · **Started:** 2026-07-24 · **Current baseline:** v2.3.0 (Python 3.8+, Rich, boto3)

---

## How to use this document

This is a **living decision log**, not a spec. It exists to (a) enumerate every requirement and open question up front, (b) give each open question a stable ID so we can discuss them one at a time without losing the thread, and (c) capture the decision and its rationale once made.

We work through it in three tracks, in this order:

| Order | Track | Why this order |
|---|---|---|
| **1st** | **A — Tech Stack, UI & UX** | The language/framework choice gates how everything else is written. |
| **2nd** | **B — Auth & Account Discovery** | Determines the data model (profiles-as-identity → accounts-as-data) that all views render. |
| **3rd** | **C — Functionality, Output & Agent Interface** | Builds on the stack and the data model. Includes a **feature adequacy review** (§C.0): are today's features actually sufficient for FinOps practice, and what should be added to drive impactful decisions? |

**Legend:** 🟢 Decided · 🟡 Open — needs discussion · 🔵 Deferred — decide after a dependency · ⚪ Out of scope for v3

Each requirement is `R-<track><n>`; each open decision is `D-<track><n>`. Decisions get recorded in the [Decision Log](#decision-log) at the bottom with date and rationale.

---

## 0. Settled premises

These came out of the initial discussion and are treated as given. They are revisitable, but not open questions.

| ID | Premise |
|---|---|
| P1 | The tool stays **CLI-native**. Not a web app, not a SaaS, not a hosted service. |
| P2 | It must **not become a heavy, loaded TUI**. Lightweight, beautiful, fast. |
| P3 | **Core FinOps features are preserved**: cost analysis, budgets, EC2 status, cost trends, FinOps audit. |
| P4 | **Cost / audit / trend remain independently runnable** — one concern at a time, as today. |
| P5 | The tool must be **usable by non-engineers**, not only engineers. |
| P6 | It must be **agent-friendly** — usable standalone *and* drivable by any agent. |
| P7 | **More AWS auth paths** are required; CLI profiles alone are not how prod works. |
| P8 | **Long flag combinations are replaced** by in-screen actions on the relevant view. |
| P9 | **PDF export is dropped** (low value, heavy dependency). |
| P10 | Reporting shifts toward **rich notifications** (Slack, Teams, email) over file artifacts. |
| P11 | The **FinOps domain logic is the asset to preserve** — Cost Explorer date semantics, tag filtering, budget parsing, resource sweeps. The CLI/render/export layers are replaceable. |

---

## 1. Guiding principles (proposed — confirm or amend)

| ID | Principle | Implication |
|---|---|---|
| G1 | **Progressive disclosure by TTY** | Interactive when attached to a terminal; static, pipeable, ANSI-free when not. One render model, two presentations. |
| G2 | **One core, many faces** | A single engine behind CLI, MCP, and Skill surfaces. No parallel implementations. |
| G3 | **Every interactive action is expressible as a command** | Interactive use teaches the flags and yields reproducible invocations. |
| G4 | **Read-only by default** | The tool never mutates AWS resources. Safe to hand to an agent or a junior engineer. |
| G5 | **API spend is a first-class concern** | Cost Explorer calls cost real money. Estimate before spending; cache aggressively. |
| G6 | **Insight over data** | Deltas, anomalies, and rankings — not raw numbers the user has to interpret. |
| G7 | **Errors are road signs** | Every failure states what happened, why, and the next action to take. |
| G8 | **Every finding is actionable** | A finding carries a dollar figure, an owner, and a rank. A list of resource IDs is homework, not a recommendation. |
| G10 | **No continuity debt to v2's UI** | The new interface owes nothing to v2's presentation. Where a design choice is incidental, prefer what the component library does well over a predetermined layout — see §A.4 for what is load-bearing vs. flexible. |
| G9 | **Financially correct by default** | Cost metric, credit/refund handling, and amortization must match what a practitioner would defend in a finance review. |

---

# TRACK A — Tech Stack, UI & UX

*Discussion order: 1st*

## A.1 Requirements

### Distribution & runtime
| ID | Requirement | Priority |
|---|---|---|
| R-A1 | Installable by a non-engineer in one step, no language runtime to manage | Must |
| R-A2 | Runs in containers and CI without a runtime install | Must |
| R-A3 | Cold start fast enough to feel instant (target < 200 ms to first paint) | Must |
| R-A4 | Works on macOS, Linux, Windows | Must |
| R-A5 | Existing install paths (pipx/pip) either preserved or given a documented migration | Should |

### Interaction model
| ID | Requirement | Priority |
|---|---|---|
| R-A6 | `finops` with no arguments does something useful — not a help dump | Must |
| R-A7 | Small memorable verb set; no long flag chains required for common tasks | Must |
| R-A8 | In-view actions to change period, grouping, filters, and to drill down | Must |
| R-A9 | Discoverable command surface (palette and/or inline help) — not keybinding memorization | Must |
| R-A10 | Footer or equivalent shows the command form of current view state (per G3) | Should |
| R-A11 | Drill-down: account → service → usage type | Should |
| R-A12 | `--watch` / live refresh | Could |

### Presentation
| ID | Requirement | Priority |
|---|---|---|
| R-A13 | Non-TTY output is clean, stable, ANSI-free and pipeable | Must |
| R-A14 | Banner, spinners, and animation are TTY-gated | Must |
| R-A15 | Honors `NO_COLOR`, `TERM=dumb`, `--plain` | Must |
| R-A16 | Deltas/anomalies visually encoded (MoM %, direction, magnitude) | Must |
| R-A17 | Readable in both light and dark terminals; degrades on 16-color | Must |
| R-A18 | Inline sparklines for trend-at-a-glance without switching views | Should |
| R-A19 | Plain-language summary line for non-engineers (deterministic, computed) | Should |
| R-A20 | Accessible: not color-alone for meaning; screen-reader-tolerable plain mode | Should |

### Onboarding
| ID | Requirement | Priority |
|---|---|---|
| R-A21 | First-run flow: detect credentials → assist login → discover accounts → write config | Must |
| R-A22 | Preflight/`doctor` command reporting identity, permissions, and available features | Must |
| R-A23 | Estimated API cost shown before a run that will incur charges | Must |
| R-A24 | Empty and error states suggest the next command | Should |

## A.2 Open decisions

| ID | Decision | Options on the table |
|---|---|---|
| 🟡 **D-A1** | **Implementation language** | (a) Python + Textual — keeps boto3 and contributors, incremental; (b) **Go + Bubble Tea/Lipgloss** — single static binary, best distribution + aesthetics, full rewrite on `aws-sdk-go-v2`; (c) Rust + ratatui — same binary win, slowest velocity; (d) hybrid |
| 🟡 **D-A2** | **UI framework**, given D-A1 | Follows from D-A1, but the "how much TUI" question is separate — full framework vs. a thin styled-output layer plus selective interactivity |
| 🟡 **D-A3** | **How interactive is the default view?** | (a) Static render, interactivity opt-in via flag; (b) interactive alt-screen by default on TTY, static when piped; (c) inline/non-alt-screen interactivity preserving scrollback |
| 🟡 **D-A4** | **Command surface shape** | Verb list; whether a `:` palette exists; whether keybindings or palette is primary; single-key vs. chorded |
| 🟡 **D-A5** | **Verb vocabulary** | e.g. `cost`, `audit`, `trend`, `accounts`, `doctor`, `whoami`, `export`, `config` — exact set and names |
| 🟡 **D-A6** | **Binary/command name** | Keep `aws-finops`? Shorten to `finops`? Both via alias? Affects muscle memory and the PyPI/brew story |
| 🟡 **D-A7** | **Distribution channels** | Homebrew, `curl \| sh`, GitHub Releases, Docker, Scoop/winget, and whether PyPI remains |
| 🟡 **D-A8** | **Theming** | Fixed palette vs. user-configurable; how to handle 16-color and light-background terminals |
| 🟡 **D-A9** | **Non-engineer mode** | Is it a separate view/flag, or is the main view already designed for them? |
| 🔵 **D-A10** | **Migration path for v2 users** | Depends on D-A1, D-A6, D-A7 |

---

# TRACK B — Auth & Account Discovery

*Discussion order: 2nd*

## B.1 Requirements

### Credential resolution
| ID | Requirement | Priority |
|---|---|---|
| R-B1 | Support the standard AWS credential chain: env vars → SSO → shared config profiles → IMDS/ECS/EKS | Must |
| R-B2 | **AWS SSO / IAM Identity Center** as a first-class path, including token-expiry detection with actionable remediation | Must |
| R-B3 | Runtime `AssumeRole` by ARN, with external ID and session-name support | Must |
| R-B4 | Ambient/instance credentials (EC2 IMDS, ECS task role, EKS IRSA / Pod Identity) — no profile required | Must |
| R-B5 | OIDC web identity (GitHub Actions, GitLab CI) | Must |
| R-B6 | Static credentials via environment variables | Must |
| R-B7 | Named profiles continue to work, unchanged, for existing users | Must |
| R-B8 | MFA prompt handling where the credential path requires it | Should |
| R-B9 | Role chaining (`role_arn` + `source_profile`) | Should |
| R-B10 | Credentials are never written to disk by this tool; secrets never logged or exported | Must |

### Account discovery & the data model
| ID | Requirement | Priority |
|---|---|---|
| R-B11 | **AWS Organizations discovery** — enumerate member accounts from the management/payer account | Must |
| R-B12 | **Payer-level Cost Explorer with `GroupBy=LINKED_ACCOUNT`** — all accounts' costs in one call | Must |
| R-B13 | Fan-out `AssumeRole` into member accounts with a configurable common role name, for resource-level (audit/EC2) data | Must |
| R-B14 | Graceful partial results — one inaccessible account degrades, never aborts | Must |
| R-B15 | Account **names** (not just 12-digit IDs) in all output | Must |
| R-B16 | Account filtering/selection: include/exclude, and by OU | Should |
| R-B17 | Account metadata cached to avoid re-enumeration on every run | Should |
| R-B18 | Multi-profile mode retained for users without Organizations access | Must |

### Identity transparency
| ID | Requirement | Priority |
|---|---|---|
| R-B19 | A command that reports resolved identity, resolution method, and account/role | Must |
| R-B20 | Permission preflight: which required IAM actions are held, and which features are consequently unavailable | Must |
| R-B21 | Documented least-privilege IAM policy (and ideally a ready-to-deploy CFN/Terraform snippet) | Should |

## B.2 Open decisions

| ID | Decision | Options on the table |
|---|---|---|
| 🟡 **D-B1** | **Is org-mode the default or opt-in?** | Auto-detect payer capability and prefer it, vs. explicit `--org`. Changes first-run behavior and cost profile significantly |
| 🟡 **D-B2** | **Primary data model** | (a) accounts-as-data with credentials as a detail (org-native); (b) profiles-as-identity (status quo); (c) unified "target" abstraction covering both |
| 🟡 **D-B3** | **Member-account role convention** | Require a named role (`OrganizationAccountAccessRole`? a custom `FinOpsReadOnly`?), configurable, with what default |
| 🟡 **D-B4** | **How far to go on SSO ergonomics** | Detect-and-instruct vs. actually triggering the SSO login flow from inside the tool |
| 🟡 **D-B5** | **Credential caching** | Cache assumed-role credentials between runs? Where, with what permissions, honoring what TTL |
| 🟡 **D-B6** | **Config file model** | Keep TOML/YAML/JSON all three, or converge on one? Where does it live (`~/.config/finops/`)? What's the precedence chain? |
| 🟡 **D-B7** | **Multi-account without Organizations** | Keep `--profiles`/`--all`/`--combine` as-is, or reshape into the target abstraction from D-B2 |
| 🟡 **D-B8** | **Cost data caching** | On-disk cache of Cost Explorer responses (per G5) — scope, TTL, invalidation, and whether it's default-on |
| 🔵 **D-B9** | **Non-AWS providers** | Is a provider abstraction worth designing for now, or explicitly AWS-only for v3? |
| 🟡 **D-B11** | **NEW — account discovery screen** (proposed 2026-07-26, deferred) | On TUI launch: a loading state while credentials resolve and accounts are enumerated, then the discovered accounts with a selection list, remembered thereafter. **Auto-advance when there is nothing to decide** — one account and valid credentials skips straight through, or it becomes the same toll booth `D-B4` rejected. Distinct from the auth screen I argued against: that one confirmed identity (no information), this one presents a real choice. **It also settles scope for the audit** — accounts chosen here, regions auto-detected, so pressing `r` on a check needs to ask nothing and the run can be quoted immediately. |
| 🟡 **D-B12** | **NEW — exploit SSO's native multi-account access** | **A single IAM Identity Center session grants access to many account+role pairs** — that is the point of Identity Center — and `sso:ListAccounts` returns them. We currently read only token *expiry* from the SSO cache. Calling it would give an engineer at a 40-account company all 40 accounts from one `aws sso login`, with **no Organizations access and no N profile entries**. This is a bigger win than org-mode fan-out for the common case, and it is the thing that makes `D-B11` worth building for SSO users rather than only for management accounts. Needs the SSO portal API client, which is new surface. |

---

# TRACK C — Functionality, Output & Agent Interface

*Discussion order: 3rd*

## C.0 Feature adequacy review

**The question:** today's feature set answers *"what did I spend?"* Practitioners are paid to answer *"what should I do about it, and what's it worth?"* This section asks whether the current features are sufficient, and what to add.

### C.0.0 Framework alignment — current state (checked 2026-07-25)

The framework has moved on from the *Inform → Optimize → Operate* phase model, and the table below still uses that older shape. **Confirmed** from [finops.org](https://www.finops.org/insights/2026-finops-framework/):

- Structure is **Domains → Capabilities → Scopes**. A Scope is a "defined segment of spending across technology categories, aligned to business constructs," determining which personas and capabilities apply.
- **Technology Categories:** Public Cloud, SaaS, Data Center, Data Cloud Platforms, AI. The discipline now explicitly extends past cloud — mission changed from *"Value of Cloud"* to *"Value of Technology."*
- **New capability:** *Executive Strategy Alignment* (in the **Manage the FinOps Practice** domain), covering Executive Priority Alignment, Multi-Year Investment Strategy, Product Prioritization, and Strategic Decision Support.
- **Renames:** Workload Optimization → **Usage Optimization** · Policy & Governance → **Governance, Policy & Risk** · FinOps Tools & Services → **Automation, Tools & Services** · Benchmarking → **KPI & Benchmarking** · Architecting for Cloud → **Architecting & Workload Placement** · Cloud Sustainability → **Sustainability**

**Still unverified:** the complete Domain list. Search summaries claim six domains (*Plan & Forecast, Inform, Optimize, Govern, Manage, Build*), but that conflicts with the confirmed *"Manage the FinOps Practice"* naming, which matches the earlier four-domain model. **To do at Track C:** confirm the domain/capability list from the authoritative framework pages and re-map the table below.

**Two implications for scoping, independent of naming:**
1. *Usage Optimization* and *KPI & Benchmarking* — precisely the two areas where this tool is emptiest (§C.0.2 Tier 1 and Tier 4) — both got attention in the 2026 revision. The gaps identified below track where the discipline is actively moving, not a personal preference.
2. The Technology Category expansion (AI spend especially) is a live question for **D-B9**: an AWS-only tool is a narrowing scope by the framework's own definition. Bedrock/SageMaker cost visibility may deserve first-class treatment rather than being just another service line.

GAP-1, GAP-2, and GAP-3 are framework-independent and stand regardless of naming.

Mapped to the FinOps Foundation's framework, v2.3.0 sits almost entirely in the **Inform** phase, and shallowly:

| Phase | Capability | v2.3.0 coverage |
|---|---|---|
| Inform | Cost visibility & reporting | 🟢 Good — periods, services, accounts, trends |
| Inform | Allocation / showback / chargeback | 🟡 Partial — tag *filtering* exists; no allocation view, no unallocated-cost KPI |
| Inform | Forecasting | 🔴 None |
| Inform | Budget management | 🟡 Reads budgets; no burn-down, no forecast-vs-budget |
| Inform | Anomaly management | 🔴 None |
| Inform | Unit economics | 🔴 None |
| Optimize | **Rate optimization** (RI/SP) | 🔴 **None — the largest dollar gap** |
| Optimize | **Usage optimization** (rightsizing) | 🟡 Detects idle EBS/EIP/stopped EC2; no rightsizing, no dollar values |
| Optimize | Commitment management | 🔴 None |
| Operate | KPIs & scorecards | 🔴 None |
| Operate | Governance / policy checks | 🟡 Untagged detection only |

### C.0.1 Three structural gaps (worth more than any single new feature)

| ID | Gap | Why it matters |
|---|---|---|
| **GAP-1** | **Findings carry no dollar value, owner, or rank** | The audit outputs lists of resource IDs. A practitioner can't take that to a decision meeting. Attaching estimated monthly savings, the owning team (from tags), and a savings-ranked order converts the same data from a report into a prioritized action list. Highest leverage change in the entire overhaul. |
| **GAP-2** | **`UnblendedCost` is hardcoded** (verified: all 15 CE call sites) | For any org with RIs or Savings Plans, unblended cost shows commitment purchases as a spike on the purchase date rather than amortized across the term — so trends and MoM deltas are actively misleading. No way to select `AmortizedCost` / `NetAmortizedCost`, and no credit/refund/tax handling. This is a correctness defect, not a missing feature. |
| **GAP-3** | **Untagged *resources* are reported; unallocated *cost* is not** | "You have 47 untagged instances" is far weaker than "18% of your spend — $14,200/mo — cannot be attributed to a team." The dollar-denominated version is a standard FinOps KPI and drives tagging-policy decisions. |

### C.0.2 Candidate additions

Grouped by value. "API" notes the data source; CE calls cost ~$0.01 each, Compute Optimizer and Cost Optimization Hub are free.

**Tier 1 — high dollar impact, moderate effort**

| ID | Feature | API / source |
|---|---|---|
| R-C30 | **RI & Savings Plan coverage %** — how much eligible spend is on-demand | `ce:GetReservationCoverage`, `ce:GetSavingsPlansCoverage` |
| R-C31 | **RI & SP utilization %** — are commitments being wasted | `ce:GetReservationUtilization`, `ce:GetSavingsPlansUtilization` |
| R-C32 | **Commitment purchase recommendations** with estimated savings | `ce:GetSavingsPlansPurchaseRecommendation` |
| R-C33 | **Expiring commitments** — what lapses in the next 30/60/90 days | RI/SP describe APIs |
| R-C34 | **Cost Optimization Hub** — AWS's consolidated, dollar-quantified recommendation set (rightsizing, idle, commitments, storage) across the org | `cost-optimization-hub:ListRecommendations` — free, org-wide, requires opt-in |
| R-C35 | **Compute Optimizer** rightsizing for EC2, EBS, Lambda, RDS, ECS + idle findings | `compute-optimizer:Get*Recommendations` — free, requires opt-in |
| R-C36 | **Month-end forecast vs. budget** — projected spend and burn-down rate | `ce:GetCostForecast` |
| R-C37 | **Cost anomalies** — surface AWS's own detections rather than reinventing | `ce:GetAnomalies` |

**Tier 2 — allocation, accountability, correctness**

| ID | Feature | Notes |
|---|---|---|
| R-C38 | **Selectable cost metric** — unblended / amortized / net amortized, with an opinionated default | Resolves GAP-2 |
| R-C39 | **Credit, refund, tax, and marketplace handling** — include/exclude, explicitly stated | Practitioners report net figures |
| R-C40 | **Unallocated cost KPI** — % and $ of spend not attributable to a tag/OU | Resolves GAP-3 |
| R-C41 | **Showback / chargeback view** — cost by team, OU, environment, project; ranked | Core practitioner deliverable |
| R-C42 | **Tagging coverage KPI** — by resource count *and* by dollar |  |
| R-C43 | **Shared-cost allocation** — split shared/untagged spend by a chosen rule | Advanced; may defer |

**Tier 3 — expanded waste detection** (extends the current audit)

| ID | Feature |
|---|---|
| R-C44 | Idle/underutilized RDS instances |
| R-C45 | gp2 → gp3 EBS migration opportunities |
| R-C46 | Orphaned EBS snapshots and AMIs |
| R-C47 | Idle NAT gateways, load balancers with no targets, unattached ENIs |
| R-C48 | S3 storage-class / lifecycle opportunities (Storage Lens) |
| R-C49 | Idle/unused Elastic IPs, old-generation instance families |

**Tier 4 — practitioner-grade reporting**

| ID | Feature | Notes |
|---|---|---|
| R-C50 | **FinOps KPI scorecard** — one screen: tagging coverage, commitment coverage, utilization, Effective Savings Rate, waste $, unallocated %, forecast vs. budget | This is what gets reported upward; nothing equivalent exists today |
| R-C51 | **Effective Savings Rate (ESR)** — the headline FinOps KPI |  |
| R-C52 | **Unit economics** — cost per user-supplied denominator (customer, transaction, environment) | Requires external input |
| R-C53 | **Period-over-period variance explanation** — *which* services/accounts drove a change, ranked by contribution | Turns "spend rose 18%" into "because of X" |

## C.1 Requirements

### Analysis features (carried forward + extended)
| ID | Requirement | Priority |
|---|---|---|
| R-C1 | Cost by period: current/previous month, custom day ranges, previous calendar month | Must |
| R-C2 | Cost by AWS service, ranked | Must |
| R-C3 | Cost by cost allocation tag, multiple tags | Must |
| R-C4 | AWS Budgets: limits vs. actuals, breach status | Must |
| R-C5 | EC2 instance status by state | Must |
| R-C6 | Cost trend over 6 months (configurable window?) | Must |
| R-C7 | FinOps audit: untagged (EC2/RDS/Lambda/ELBv2), stopped EC2, unused EBS, unused EIPs, budget breaches | Must |
| R-C8 | Month-over-month deltas and anomaly flagging | Must |
| R-C9 | Cost by account (org-native view) | Must |
| R-C10 | Drill-down to usage type / region granularity | Should |
| R-C11 | Estimated savings attached to audit findings | Should |

### Output & delivery
| ID | Requirement | Priority |
|---|---|---|
| R-C12 | JSON output on every command, stable versioned schema | Must |
| R-C13 | Markdown output (human report + chat-postable + agent-cheap) | Must |
| R-C14 | Rich native Slack messages (Block Kit), not just file uploads | Must |
| R-C15 | Microsoft Teams (Adaptive Cards) | Must |
| R-C16 | Email delivery | Should |
| R-C17 | Generic webhook sink | Could |
| R-C18 | S3 destination retained | Should |
| R-C19 | Trend data exportable in all formats (today: JSON only — a known gap) | Should |
| R-C20 | Secrets (Slack/Teams/SMTP tokens) via env or secret store only — never config file or flag | Must |

### Agent interface
| ID | Requirement | Priority |
|---|---|---|
| R-C21 | Structured errors on stderr with `code`, `message`, `remediation` | Must |
| R-C22 | Meaningful exit-code taxonomy (ok / usage / auth / permission / no-data / findings-exist) | Must |
| R-C23 | Capability discovery command (`schema` / `--agent-info`): verbs, flags, output schemas, API cost | Must |
| R-C24 | Token-efficiency controls: `--top N`, field selection, compact JSON | Must |
| R-C25 | `--explain-cost` / `--dry-run` — API calls and dollar estimate before spending | Must |
| R-C26 | Deterministic, idempotent, side-effect-free reads | Must |
| R-C27 | Published Agent Skill (`SKILL.md`) describing procedural usage | Should |
| R-C28 | Sister MCP server refactored to a thin shim over the same core (per G2) | Should |
| R-C29 | Threshold checks with non-zero exit for CI guardrail use | Should |

## C.2 Open decisions

| ID | Decision | Options on the table |
|---|---|---|
| 🟡 **D-C1** | **Does CSV survive?** | Your instinct was to drop it; the counter-argument is that it's the main non-engineer/Excel bridge and is nearly free once a table model exists. Needs resolution |
| 🟡 **D-C2** | **Format × destination matrix** | Which formats are valid to which sinks, and is that orthogonal or constrained? |
| 🟡 **D-C3** | **Notification content design** | Full data in-message vs. summary + attached detail vs. summary + link |
| 🟡 **D-C4** | **Scheduling** | Does the tool schedule its own recurring notifications, or stay stateless and defer to cron/CI/Lambda? |
| 🟡 **D-C5** | **JSON schema design** | Envelope shape, versioning policy, and how stability is guaranteed to agents over time |
| 🟡 **D-C6** | **Any LLM features in-tool?** | Optional natural-language summary/query, or strictly deterministic with intelligence left to the calling agent? |
| 🟡 **D-C7** | **MCP server relationship** | Fold the sister project in as a subcommand (`finops mcp`), keep it separate but library-backed, or deprecate it in favor of CLI-invocation? |
| 🟡 **D-C8** | **Audit scope expansion** | More resource types (idle RDS, gp2→gp3, orphaned snapshots, old NAT gateways, unattached ENIs)? Where's the line for v3? |
| 🟡 **D-C9** | **Savings estimation** | Do we compute dollar savings for findings, and with what pricing source? |
| 🟡 **D-C10** | **Trend window** | Fixed 6 months or configurable? Granularity — monthly only, or daily/weekly? |
| 🟡 **D-C11** | **Anomaly detection method** | Simple threshold/percentage, statistical, or surface AWS Cost Anomaly Detection findings? |
| 🔵 **D-C12** | **Reserved Instance / Savings Plan coverage & utilization** | Superseded by D-C14 — see §C.0.2 Tier 1 |
| 🟡 **D-C13** | **Default cost metric** (GAP-2) | Unblended (status quo, wrong for commitment-heavy orgs) vs. amortized vs. net amortized as default. And: is it selectable, or opinionated-and-fixed? |
| 🟡 **D-C14** | **How far up the FinOps maturity curve does v3 go?** | (a) Inform done excellently — visibility, allocation, forecast, anomalies; (b) Inform + Optimize — add rate/usage optimization and recommendations; (c) all three phases incl. KPI scorecard. Determines scope more than any other decision in Track C |
| 🟡 **D-C15** | **Build recommendations or surface AWS's?** | Cost Optimization Hub and Compute Optimizer already compute dollar-quantified recommendations for free. Surfacing them is cheap and authoritative; computing our own is differentiating but costly and disputable. Likely a mix — where's the line? |
| 🟡 **D-C16** | **Do audit findings get dollar values?** (GAP-1) | Requires a pricing source: Pricing API, CUR, Cost Optimization Hub's own estimates, or heuristics. Big usability win, real complexity |
| 🟡 **D-C17** | **Data source strategy** | Cost Explorer API only (easy, $0.01/call, limited granularity) vs. optionally CUR 2.0 / Data Exports via Athena (free at scale, resource-level detail, requires setup). Does v3 support a CUR backend? |
| 🟡 **D-C18** | **FOCUS conformance** | Should JSON output conform to the FinOps Foundation's FOCUS spec? Strong interop/credibility signal, constrains schema design (interacts with D-C5) |
| 🟡 **D-C19** | **Opt-in-dependent features** | Cost Optimization Hub and Compute Optimizer require account-level enablement. How do we detect, communicate, and degrade when they're off? |
| 🟡 **D-C20** | **Credit/refund/tax handling** | Include or exclude by default, and how is the choice surfaced so numbers are defensible? |
| 🟡 **D-C21** | **Is there a KPI scorecard view?** | A dedicated verb (`finops kpi`?) vs. folded into the cost view vs. out of scope |

---

# Cross-cutting

| ID | Decision | Notes |
|---|---|---|
| 🟡 **D-X1** | **Architecture layering** | Proposed: `core` (auth, providers, models) → `engine` (queries → typed results) → `render` (tui/plain/json/markdown/csv) → `sink` (stdout/file/s3/slack/teams/email) → `faces` (cli/mcp/skill). Confirm or amend |
| 🟡 **D-X2** | **Repo strategy** | New repo, or v3 branch in this one? Does v2 get maintained in parallel, and for how long? |
| 🟡 **D-X3** | **Versioning & breaking changes** | Is v3 explicitly breaking? What's the deprecation communication plan? |
| 🟢 **D-X4** | **Testing: TDD for the engine, snapshots for the UI** | **TDD applies to the engine** — cost date arithmetic, pricing calculations, check evaluators (API response → findings), formatters. These are input→output functions, and TDD matters more than usual here because the spike's four real bugs were all *semantically wrong but cleanly typed*: exactly what a test catches and a compiler does not. **TDD does not apply to the TUI** — you cannot write a meaningful failing test for "the table looks right" before the screen exists; golden-frame snapshots are characterization tests written *after*, and forcing TDD there produces brittle tests that assert implementation detail. **Caveat:** for unfamiliar Charm APIs, spike → verify against source → *then* TDD the surrounding logic; writing tests against a guessed API only relocates the guess. **Stack:** stdlib `testing` with table-driven tests (no testify — fewer deps, more legible for review); narrow per-call interfaces with hand-written fakes for AWS; pty + pyte golden frames for screens; language-neutral JSON fixtures for v2 parity. | 2026-07-26 |
| 🟡 **D-X5** | **Telemetry** | Any usage analytics? (Default-off, opt-in, or never) |
| 🟡 **D-X6** | **Docs** | Where do docs live, and what's the minimum set for launch? |
| 🟡 **D-X7** | **Backward compatibility of v2 flags** | Accept-and-translate old flags, hard break, or a deprecation window? |
| 🟢 **D-X8** | **AI-native maintainability scaffolding — build BEFORE feature code** | See §X.1. Decided 2026-07-25. |
| 🟢 **D-X9** | **Spec-driven for exactly two things; no ceremony elsewhere** | **(1) The JSON output schema** — a contract with agents, painful to change later. Define as an actual schema file and validate output against it in CI (ties to `D-C5`, `D-C18`/FOCUS). **(2) Check declarations as data, not code** — each audit check declared once with name, category, basis, description, required API calls, thresholds, and evaluator; the catalogue UI, settings screen, scan planner, docs, and `finops schema` output all *derive* from that declaration. Same principle that makes `key.Binding` → `help` self-syncing (`D-A4`): declare once, generate the rest. **Explicitly not spec-driven:** no formal spec document per increment — the design conversation plus mockups is the spec, and this decision log is the durable record. Per-increment planning stays lightweight: agreed mockup + task list + the tests it must pass. | 2026-07-26 |

### X.1 D-X8 — AI-native maintainability scaffolding

**Premise (`P12`):** AI is the primary author, and future sessions may use models with *older* knowledge than today's. The scaffolding that catches AI error must therefore exist *before* the code it needs to catch.

**Governing principle:** prefer mechanisms that require the model to know, remember, or believe nothing. A failing test beats a document, because it works on a model that never reads it.

**Evidence this is necessary.** Five knowledge gaps hit during the 2026-07-25 spike, and how each was actually resolved:

| Gap | Web search | Source read | Execution |
|---|---|---|---|
| `Model.View()` returns `View`, not `string` | vague | ✅ | — |
| `LightDark()` exists in Lipgloss v2 | ❌ **misled** | ✅ | — |
| `Compositor`, not `Canvas.Compose` | ❌ no coverage | ✅ answer | ✅ **detected** |
| Ink v7 *has* absolute positioning | ❌ no coverage | ✅ | — |
| Absolute positioning doesn't occlude | ❌ no coverage | ❌ **can't tell** | ✅ |

Search resolved **0 of 5**. Source resolved 3. Two required running code. **Critically: no compiler caught any of the four real bugs — all were semantically wrong and cleanly typed.** Search detects *staleness*; source gives the *contract*; only execution reveals *behaviour*. Search and source are free; **execution is the tier that must be built.**

**Tier 1 — mechanical (works on any model)**
1. **Golden-frame snapshot tests.** Promote the spike's `drive.py` (pty + `TIOCSWINSZ` + timed keystrokes) and `frame.py` (pyte terminal emulation) into the repo. Every screen gets a committed expected frame; CI drives the TUI and diffs. This is the only mechanism that catches "compiles, renders *almost* right".
2. **Characterization tests on v2 domain logic before porting** — especially Cost Explorer date arithmetic and `last-month` semantics (fixed in #48). Capture current behaviour as fixtures first, port against them.
3. **`make verify`** — build, vet, test, snapshot-diff, smoke-run. One command, unambiguous pass/fail, requires no project understanding.

**Tier 2 — in-repo knowledge (works if read)**
4. **`docs/STACK.md`** — a dated, version-stamped ledger of *verified* API facts, weighted toward things that **compile but misbehave**. Every entry carries the version, the date, and the command to re-verify. Seed it with all five gaps above.
5. **`CLAUDE.md` verification protocol** — behavioural: *"Charm v2 shipped 2026-07-03; your training is likely v1-shaped and v1 idioms compile and misbehave. Before using any Charm API not already used in this session, run `go doc` or read `$GOMODCACHE`. Do not rely on recall. Check `docs/STACK.md` first."*
6. **Runnable examples for tricky APIs** — the spike's `composetest` becomes `examples/compositing/`. Executable beats prose: a model can copy it *and* run it.

**Tier 3 — environment**
7. **`go mod vendor`** — real dependency source in-tree, readable offline with no network tool. Practical because Go's dependency count is small (29 modules).
8. **Pin the toolchain**, Go version included.
9. **No automated dependency upgrades.** Renovate/Dependabot silently bumping Charm desynchronises `STACK.md` from reality and reintroduces the exact gap this defends against. Upgrades are deliberate commits touching code, `STACK.md`, and snapshots together.

**Tier 4 — make the error unrepresentable**
10. **Encode knowledge in the type system, not comments.** A model cannot misuse an API that does not exist: keep footgun primitives unexported behind safe constructors, and make required context (e.g. the cost metric on any rendered figure) a mandatory struct field rather than a documented convention.

---

## Explicitly out of scope for v3

| ID | Item | Why |
|---|---|---|
| ⚪ O1 | Hosted/SaaS version, web UI | Contradicts P1 |
| ⚪ O2 | Write actions against AWS (stopping instances, deleting volumes, applying tags) | Contradicts G4 |
| ⚪ O3 | Historical data warehouse / long-term storage of cost data | Scope; CUR + Athena is the right tool |
| ⚪ O4 | Multi-cloud (Azure, GCP) | See D-B9 — provider abstraction may be designed for, but not implemented |

---

## Decision Log

Filled in as we resolve each item.

| ID | Decision | Rationale | Date |
|---|---|---|---|
| **P12** | **This is an AI-native project.** AI is the primary author and maintainer; the owner reviews behaviour and structure rather than syntax. | Stated by the owner, 2026-07-25. **Load-bearing premise for D-A1** — the case for Go rests entirely on it. If this stops being true, reopen D-A1. | 2026-07-25 |
| **D-A1** | **Go**, with `aws-sdk-go-v2` | Decided by building the same dashboard screen twice — measurements in §A.3. **Python eliminated by requirement:** 140–170 ms bare interpreter, 300–470 ms with boto3, ~400 ms to a first CE client, against a <200 ms `R-A3` budget; and Rich has no input handling and no compositing, so the interactive model is unbuildable on it. **TS + Ink was chosen, then reversed.** It was chosen on maintainer fluency (owner works professionally in TS/React) — a difference in kind, not degree, for a solo-maintained project. **P12** withdrew that premise: with AI as primary author, fluency is no longer binding, and Go's measured advantages become available. Go wins on first paint **21 ms vs 85 ms**, binary **5.6 MB vs 62 MB**, **0 build-config workarounds vs 3**, **29 modules vs 43 packages / 70 MB**, native overlay compositing with z-index and hit testing, a stricter compiler (unused variables are errors; no `any` escape hatch), canonical `gofmt`, and goroutine-based org fan-out for `R-B13`. Both readings of "AI-native" agree — a 5.6 MB static binary with 21 ms cold start is also strictly better for **agent** invocation (`R-C21`–`R-C26`) than a 62 MB artifact requiring Node. | 2026-07-25 |
| **D-A2** | **Charm v2 — Bubble Tea, Lipgloss, Bubbles, Glamour**, pinned | Import paths `charm.land/{bubbletea,bubbles,lipgloss}/v2`; resolved to bubbletea **v2.0.8**, lipgloss **v2.0.5**, bubbles **v2.1.1**. **14 first-party components, ~13,000 lines** (textarea, list, viewport, table, textinput, filepicker, progress, paginator, help, cursor, spinner, timer, stopwatch, key) versioned in lockstep with the framework — versus Ink's 6 primitives plus a third-party kit stale since May 2024 (`@inkjs/ui`) and Dec 2023 (`ink-table`). Glamour covers Markdown rendering for `R-C13`. **Risk accepted:** v2 shipped 2026-07-03 and AI training knowledge is v1-shaped; the spike confirmed this bites. **Mandatory mitigation: verify every Charm v2 API against module source or pkg.go.dev before use — never rely on recall.** See `D-X8`. | 2026-07-25 |
| **D-A3** | ⚠️ **REVERSED 2026-07-26 — alt-screen for the TUI; non-TUI output unaffected** | *Originally: inline rendering throughout, on the reasoning that scrollback preservation makes a tool feel like a command rather than an application.* **Reversed after running it.** The owner ran the credential picker and could scroll the terminal away from it — which exposed a contradiction: we had designed top bars, bottom bars, fixed height, and vertical centring, all of which are application conventions. A surface that looks like a screen but scrolls like output is a mixed signal. **Resolution:** the TUI takes the alternate screen and restores the terminal on exit; anything worth keeping comes from `--format plain` or `--format json`. Exploring and recording are different jobs, which the three-mode model already assumes. **Implementation note:** in Bubble Tea v2 alt-screen is a *declarative field on `tea.View`* (`v.AltScreen = true`), not a program option — `tea.WithAltScreen()` is a v1 idiom and does not compile. Also capped vertical centring at 6 lines: true centring strands a short menu mid-void on a tall terminal. | 2026-07-26 |
| **D-A8** | **Adaptive theming via `LightDark()` + `tea.BackgroundColorMsg`** | Lipgloss v2 removed the `AdaptiveColor` *type* but **not** adaptive theming: `lipgloss.LightDark(isDark)(light, dark)` selects per-colour, and Bubble Tea delivers `tea.BackgroundColorMsg` (with `IsDark()`) through `Update` on start and on change. Detection happens once via a message rather than lazily per colour. *(Corrects an earlier claim in this doc that v2 removed adaptive colour entirely — that was wrong, derived from a search summary and refuted by reading the API.)* Ship one curated palette for v3.0, defer user themes, and encode meaning with colour **and** symbol (▲▼) per `R-A20`. **Palette decided 2026-07-26 — see §A.5.** | 2026-07-25 |
| **D-A4** | **Interactivity architecture — Charm components, verified against installed source** | **Confirmed by owner 2026-07-26.** Component mapping, all first-party unless noted: **Command palette** = `bubbles/list` (built-in ranked fuzzy filter: `FilterFunc`, embedded `FilterInput textinput.Model`, `FilterState`, `DefaultFilter`/`UnsortedFilter`) + `bubbles/textinput` suggestion engine (`SetSuggestions`, `MatchedSuggestions`, `CurrentSuggestion`). **Footer + `?` overlay** = `bubbles/key.Binding` with `WithHelp(key, desc)` → `bubbles/help.Model.View(keyMap)`; `ShortHelp()` renders the footer, `FullHelp()` the grouped overlay, **from the same declarations**. Disabling a binding (`SetEnabled(false)`) removes it from both automatically — the docs state help "should self-manage". This makes `G3` structural: visible commands cannot drift from real bindings. **Pickers/menus** = `bubbles/list`. **Scrolling** = `bubbles/viewport`. **Overlays** = `lipgloss.NewCompositor` (never `Canvas.Compose`). **Hand-rolled:** tabs (no Charm component; Lipgloss-border pattern) and focus management — acceptable because drill-down *replaces* views rather than tabbing across them, so only one surface is focused at a time. Verified working demo: `examples/keyhelp`. | 2026-07-26 |
| 🟡 **D-A11** | **OPEN — local persistent state store?** | Surfaced by the UX design: the audit's "▼ $1,200 vs last run", suppressed/acknowledged findings, and KPI history all require persistence. Proposal: a small store under `~/.local/state/finops/` (SQLite via a pure-Go driver, or JSON) also holding the Cost Explorer response cache from `G5`. Open questions: authoritative vs. disposable, cache invalidation, multi-machine divergence. *(This slot previously held an Ink `<Overlay>` workaround — moot under D-A1, since Lipgloss's `Compositor` occludes natively with z-index and hit testing.)* | 2026-07-25 |
| **D-B1** | **Org-first — Organizations discovery is the primary path, not opt-in** | Auto-detect: if resolved credentials belong to an Organizations management account, prefer payer-level Cost Explorer with `GroupBy=LINKED_ACCOUNT` — one call for all accounts instead of one per account — and offer member-account discovery. Rationale: a static CLI-profile-per-account is not how production organisations work; it is an individual/small-user pattern. Must show a visible org-mode indicator and support an explicit override. | 2026-07-26 |
| **D-B2** | **Accounts-as-data** — the account is the unit; credentials are a detail of how it is reached | v2's model is *one profile = one account = one row*, which requires configuring N profiles for N accounts and breaks entirely in CI, where no profile exists. Real organisations use one identity (SSO / AssumeRole / IRSA / OIDC) reaching many accounts through a single role name. The audit design depends on this — every finding carries an account, and one credential may cover fifty. `--profiles` / `--all` / `--combine` are retained but **demoted** to the individual path and the escape hatch when Organizations is unavailable. | 2026-07-26 |
| **D-B3** | **Member-account role: `FinOpsReadOnly` by default, configurable via `--org-role`, no silent fallback** | Org mode authenticates to the management account, lists members, then assumes a role in each to read resource-level data for the audit. **Rejected `OrganizationAccountAccessRole`** — AWS provisions it automatically so it needs zero setup, but it carries `AdministratorAccess`, which is indefensible for a read-only tool (`G4`) and is commonly blocked by security teams. Instead ship a **CloudFormation StackSet** deploying a least-privilege `FinOpsReadOnly` role org-wide in one action. Missing role → clear remediation error (`G7`), never a silent escalation to admin. | 2026-07-26 |
| **D-B4** | **Auth is a state shown in the header, not a screen** — and the tool *does* trigger SSO login | **Resolution chain (automatic):** env vars → SSO session → shared config profile → IRSA / ECS task role / IMDS → OIDC. First match wins; then detect whether the identity is an Organizations management account. **Three user-visible states:** (1) *works* → no auth UI, just a header strip `prod-mgmt · 123456789012 · SSO acme-sso · 6h 12m · org: 14 accounts`; (2) *expired* → a road sign, not a screen: `✗ SSO session expired 42m ago · Run: aws sso login --sso-session acme-sso`; (3) *nothing resolves / first run* → the only real flow, offering SSO (recommended, **shells out to `aws sso login`**), an existing profile, assume-role by ARN, or env vars. **Org discovery runs once** and is remembered, quoting the saving (1 call vs N) and offering `finops auth stackset` when `FinOpsReadOnly` is absent. Existing CLI profiles keep working untouched — the point is that nobody in an org needs to create fourteen of them. **Three follow-ups agreed 2026-07-26:** (a) **`finops auth` is a real command**, not just status output — `auth login` and `auth stackset` need somewhere to live; (b) **on first run with no credentials the picker *is* the first screen** rather than an error, since there is nothing useful to render without credentials; (c) **permissions are preflighted, not discovered lazily** — probe the required IAM actions once, cache the result, re-run only on `finops auth`. Preflight turns "the audit returned nothing" into "you are missing `ec2:DescribeVolumes`". | 2026-07-26 |
| **D-B5** | **Persist configuration, never credentials** | `~/.config/finops/config.toml` stores the *choice* — credential source, org mode on/off, member role name, region scope, defaults. It never stores access keys, session tokens, or SSO tokens; SSO tokens remain in the SDK's own `~/.aws/sso/cache`. Keeps `R-B10` intact. Second launch onward: resolve silently, render the header strip, land directly on the audit home. | 2026-07-26 |

### A.5 Colour system (decided 2026-07-26)

Seven semantic roles. Every value must survive 256-colour, degrade to 16, and stay legible on
**both** light and dark terminal backgrounds. Selected via `lipgloss.LightDark(isDark)` with
`isDark` arriving from `tea.BackgroundColorMsg` (`D-A8`).

| Role | Meaning | Dark bg | Light bg | ANSI-256 |
|---|---|---|---|---|
| `text` | primary content | `#e6edf3` | `#1f2328` | default |
| `dim` | labels, secondary, timestamps | `#8b949e` | `#656d76` | 245 / 243 |
| `accent` | commands, keys, links, bars | `#79c0ff` | `#0969da` | 75 / 26 |
| `positive` | cost ↓, savings available, ✓ | `#3fb950` | `#1a7f37` | 71 / 28 |
| `negative` | cost ↑, errors, breaches, ✗ | `#f85149` | `#cf222e` | 203 / 160 |
| `warning` | stale, approximate, expiring | `#d29922` | `#9a6700` | 178 / 136 |
| `selection` | highlighted row background | `#21262d` | `#eaeef2` | 236 / 254 |
| `border` | boxes, rules | `#30363d` | `#d0d7de` | 240 / 252 |

**Two rules that matter more than the values:**

1. **Never colour alone.** Every coloured signal carries a glyph — `▲▼`, `✓✗⚠`, `~` for
   approximate. Satisfies `R-A20`, and keeps screenshots legible when pasted into Slack.
2. **Apply by meaning, not by magnitude.** A FinOps-specific trap: a cost *increase* is negative
   (red) while a large *savings* figure is positive (green). Same big number, opposite valence.

Reference mockups: `docs/mockups/`.

### A.3 Spike measurements (2026-07-25)

Same `finops cost` screen — streaming rows, keyboard nav, floating period-picker overlay, inline rendering — built twice from an identical spec and mock dataset. Artifacts in `scratchpad/spike/{goapp,ink}` with a pty driver and pyte frame renderer.

| | Go + Charm v2 | TS + Ink v7 |
|---|---|---|
| First paint (warm, median of 5–6) | **21 ms** | 78 ms node · 85 ms bun binary |
| First paint (cold cache) | — | ~975 ms |
| UI code, non-blank | 263 lines | **169 lines** |
| Standalone artifact | **5.6 MB** | 62 MB (bun compile) |
| Dependencies | 29 modules | 43 packages / 70 MB |
| Build-config workarounds | **0** | 3 (devtools stub, `createRequire` banner, alias) |
| Overlay occlusion | native + z-index + hit testing | needs explicit per-node `backgroundColor` |
| Text wrapping | manual | **automatic** |

**Friction encountered.** Go: one hard problem — `Canvas.Compose(Layer)` type-checks but silently ignores the layer's X/Y *and* all child layers; `Compositor` is the real entry point, found only by reading the module source. Ink: three shallow bundling problems plus the non-obvious overlay occlusion rule, isolated by testing five positioning variants.

**Also measured:** Python startup floor on the maintainer's machine — 140–170 ms bare, 300–470 ms with boto3, ~400 ms to a Cost Explorer client. Repo stats: 106 commits, 100 by the maintainer, 6 external contributors / 12 commits; 2,911 LOC of which ~1,300 is worth porting and ~700 genuinely subtle.

---

## Status and next steps

**Track A — substantially decided** (2026-07-25). Stack, UI framework, render model, theming, and the AI-native scaffolding are settled; see the Decision Log and §A.3.

Still open in Track A, all lower-stakes and safe to settle during implementation:

| ID | Open item |
|---|---|
| `D-A4` / `D-A5` | Exact key map and verb vocabulary — proposed (`cost`/`audit`/`trend`/`accounts`/`kpi`/`doctor`/`whoami`/`config`/`schema`/`mcp`, keys + `:` palette + `?` help), not ratified. Note `export` is deliberately a sink flag, not a verb. |
| `D-A6` | Command name — proposed `finops` with an `aws-finops` alias, PyPI package name unchanged |
| `D-A7` | Distribution channels — goreleaser/GitHub Releases, Homebrew tap, `curl \| sh`, Docker, Scoop/winget. **Note:** the PyPI-wheel-wrapping-a-binary trick (the ruff/uv pattern) is what preserves existing `pipx install aws-finops-dashboard` users |
| `D-A9` | Non-engineer UX — proposed: no separate mode; legible defaults + `finops kpi` + strong Slack/Teams notifications |
| `D-A10` | v2 migration path — proposed: accept v2 flags for one minor version, printing the new equivalent |
| `D-A11` | Local persistent state store (see Decision Log) |
| §A.4 | The load-bearing vs. flexible design table that `G10` references is **not yet written** |

**Also still unanswered, and it bounds Track A's remaining scope:** the engineer-to-non-engineer ratio among current users, and any signal on the pipx/pip/Docker/source install mix (bounds `D-A10`).

**Next track: B — Auth & Account Discovery.** Start with `D-B1` (is org mode the default?) and `D-B2` (accounts-as-data vs. profiles-as-identity), since the dashboard design in this document is unbuildable on the profiles-as-identity model — every figure is per-account with a name attached.

## Preview question for Track C (no answer needed yet)

**D-C14 is the scope-defining decision of the whole project.** Going from "excellent Inform" to "Inform + Optimize" roughly doubles the API surface and adds dependencies on org-level AWS opt-ins — but it's also the difference between a tool that reports spend and a tool that changes it. Worth having a view on before we design the UI, since a recommendations-and-KPIs product needs different screens than a cost-reporting one.
