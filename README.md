# AWS FinOps Dashboard (CLI) v2.2.7

[![PyPI version](https://img.shields.io/pypi/v/aws-finops-dashboard.svg)](https://pypi.org/project/aws-finops-dashboard/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![GitHub stars](https://img.shields.io/github/stars/ravikiranvm/aws-finops-dashboard.svg)](https://github.com/ravikiranvm/aws-finops-dashboard/stargazers)
[![Downloads](https://static.pepy.tech/badge/aws-finops-dashboard)](https://pepy.tech/project/aws-finops-dashboard)

The AWS FinOps Dashboard is an open-source, Python-based command-line tool (built with the Rich library) for AWS cost monitoring. It provides multi-account cost summaries by time period, service, and cost allocation tags; budget limits vs. actuals; EC2 instance status; six‑month cost trend charts; and “FinOps audit” reports (e.g. untagged or idle resources). It can export data to CSV/JSON/PDF.

## Why AWS FinOps Dashboard?

Managing and understanding your AWS expenditure, especially across multiple accounts and services, can be complex. The AWS FinOps Dashboard CLI aims to simplify this by providing a clear, concise, and actionable view of your AWS costs and operational hygiene directly in your terminal.

Key benefits include:
*   **Unified View:** Consolidate cost and resource data from multiple AWS profiles.
*   **Actionable Insights:** Quickly identify spending patterns, untagged resources, and potential savings.
*   **Terminal-First:** Access crucial FinOps data without leaving your command line.
*   **Customizable & Extensible:** Open-source and built with Python, allowing for easy customization.

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [AWS CLI Profile Setup](#aws-cli-profile-setup)
- [Command Line Usage](#command-line-usage)
  - [Options](#command-line-options)
  - [Examples](#examples)
- [Using a Configuration File](#using-a-configuration-file)
  - [TOML Configuration Example (`config.toml`)](#toml-configuration-example-configtoml)
  - [YAML Configuration Example (`config.yaml` or `config.yml`)](#yaml-configuration-example-configyaml-or-configyml)
  - [JSON Configuration Example (`config.json`)](#json-configuration-example-configjson)
- [Export Formats](#export-formats)
- [Cost For Every Run](#cost-for-every-run)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Cost Analysis by Time Period**: 
  - View current & previous month's spend by default
  - Set custom time ranges (e.g., 7, 30, 90 days) with `--time-range` option
- **Cost by AWS Service**: Sorted by highest cost for better insights
- **Cost by Tag**: Get the cost data by one or more tags with `--tag`(cost allocation tags must be enabled)
- **AWS Budgets Information**: Displays budget limits and actual spend
- **EC2 Instance Status**: Detailed state information across specified/accessible regions
- **Cost Trend Analysis**: View detailed cost trends in bar charts for the last 6 months across AWS profiles
- **FinOps Audit**: View untagged resources, unused or stopped resources, and Budget breaches across AWS profiles. Audit reports are exported in PDF format.
- **Profile Management**:
  - Automatic profile detection
  - Specific profile selection with `--profiles`
  - Use all available profiles with `--all`
  - Combine profiles from the same AWS account with `--combine`
- **Region Control**: Specify regions for EC2 discovery using `--regions`
- **Export Options**:
  - CSV export with `--report-name` and `--report-type csv`
  - JSON export with `--report-name` and `--report-type json`
  - Export to both CSV and JSON formats with `--report-name` and `--report-type csv json`
  - Specify output directory using `--dir`
  - PDF export with `--report-name` and `--report-type pdf`
  - **Note**: Audit reports (generated via `--audit`) currently only support PDF and JSON export. Other formats specified in `--report-type` will be ignored for these reports.
- **Improved Error Handling**: Resilient and user-friendly error messages
- **Beautiful Terminal UI**: Styled with the Rich library for a visually appealing experience

---

## Prerequisites

- **Python 3.8 or later**: Ensure you have the required Python version installed
- **AWS CLI configured with named profiles**: Set up your AWS CLI profiles for seamless integration
- **AWS credentials with permissions**:
  - `ce:GetCostAndUsage`
  - `budgets:ViewBudget`
  - `ec2:DescribeInstances`
  - `ec2:DescribeRegions`
  - `sts:GetCallerIdentity`
  - `ec2:DescribeInstances`
  - `ec2:DescribeVolumes`
  - `ec2:DescribeAddresses`
  - `rds:DescribeDBInstances`
  - `rds:ListTagsForResource`
  - `lambda:ListFunctions`
  - `lambda:ListTags`
  - `elbv2:DescribeLoadBalancers`
  - `elbv2:DescribeTags`
  
---

## Installation

There are several ways to install the AWS FinOps Dashboard:

### Option 1: Using pipx (Recommended)
```bash
pipx install aws-finops-dashboard
```

If you don't have `pipx`, install it with:

```bash
python -m pip install --user pipx
python -m pipx ensurepath
```

### Option 2: Using pip
```bash
pip install aws-finops-dashboard
```

### Option 3: Using uv (Fast Python Package Installer)
[uv](https://github.com/astral-sh/uv) is a modern Python package installer and resolver that's extremely fast.

```bash
# Install uv if you don't have it yet
curl -LsSf https://astral.sh/uv/install.sh | sh

# Create project directory
mkdir aws-finops-dashboard && cd aws-finops-dashboard

# Create and activate python virtual environment
uv venv && source .venv/bin/activate

# Install aws-finops-dashboard
uv pip install aws-finops-dashboard
```

### Option 4: From Source
```bash
# Clone the repository
git clone https://github.com/ravikiranvm/aws-finops-dashboard.git
cd aws-finops-dashboard

# Create and activate python virtual environment
python -m venv .venv && source .venv/bin/activate

# Install using pip
pip install -e .
```

---

## AWS CLI Profile Setup

If you haven't already, configure your named profiles using the AWS CLI:

```bash
aws configure --profile profile1-name
aws configure --profile profile2-name
# ... etc ...
```

Repeat this for all the profiles you want the dashboard to potentially access.

---

## Command Line Usage

Run the script using `aws-finops` followed by options:

```bash
aws-finops [options]
```

### Command Line Options

| Flag | Description |
|---|---|
| `--config-file`, `-C` | Path to a TOML, YAML, or JSON configuration file. Command-line arguments will override settings from the config file. |
| `--profiles`, `-p` | Specific AWS profiles to use (space-separated). If omitted, uses 'default' profile if available, otherwise all profiles. |
| `--regions`, `-r` | Specific AWS regions to check for EC2 instances (space-separated). If omitted, attempts to check all accessible regions. |
| `--all`, `-a` | Use all available AWS profiles found in your config. |
| `--combine`, `-c` | Combine profiles from the same AWS account into single rows. |
| `--tag`, `-g` | Filter cost data by one or more cost allocation tags in `Key=Value` format. Example: `--tag Team=DevOps Env=Prod` |
| `--report-name`, `-n` | Specify the base name for the report file (without extension). |
| `--report-type`, `-y` | Specify report types (space-separated): 'csv', 'json', 'pdf'. For reports generated with `--audit`, only 'pdf' is applicable and other types will be ignored. |
| `--dir`, `-d` | Directory to save the report file(s) (default: current directory). |
| `--time-range`, `-t` | Time range for cost data in days (default: current month). Examples: 7, 30, 90. |
| `--trend` | View cost trend analysis for the last 6 months. |
| `--audit` | View list of untagged, unused resoruces and budget breaches. |

### Examples

```bash
# Use default profile, show output in terminal only
aws-finops

# Use specific profiles 'dev' and 'prod'
aws-finops --profiles dev prod

# Use all available profiles
aws-finops --all

# Combine profiles from the same AWS account
aws-finops --all --combine

# Specify custom regions to check for EC2 instances
aws-finops --regions us-east-1 eu-west-1 ap-southeast-2

# View cost data for the last 30 days instead of current month
aws-finops --time-range 30

# View cost data only for a specific tag (e.g., Team=DevOps)
aws-finops --tag Team=DevOps

# View cost data for multiple tags (e.g., Team=DevOps and Env=Prod)
aws-finops --tag Team=Devops Env=Prod

# Export data to CSV only
aws-finops --all --report-name aws_dashboard_data --report-type csv

# Export data to JSON only
aws-finops --all --report-name aws_dashboard_data --report-type json

# Export data to both CSV and JSON formats simultaneously
aws-finops --all --report-name aws_dashboard_data --report-type csv json

# Export combined data for 'dev' and 'prod' profiles to a specific directory
aws-finops --profiles dev prod --combine --report-name report --report-type csv --dir output_reports

# View cost trend analysis as bar charts for profile 'dev' and 'prod'
aws-finops --profiles dev prod -r us-east-1 --trend

# View cost trend analysis for all cli profiles for a specific cost tag 'Team=DevOps'
aws-finops --all --trend --tag Team=DevOps

# View audit report for profile 'dev' in region 'us-east-1'
aws-finops -p dev -r us-east-1 --audit

# View audit report for profile 'dev' in region 'us-east-1' and export it as a pdf file to current working dir with file name 'Dev_Audit_Report'
aws-finops -p dev -r us-east-1 --audit -n Dev_Audit_Report -y pdf

# Use a configuration file for settings
aws-finops --config-file path/to/your_config.toml
# or
aws-finops -C path/to/your_config.yaml
```

You'll see a live-updating table of your AWS account cost and usage details in the terminal. If export options are specified, a report file will also be generated upon completion.

---

## Using a Configuration File

Instead of passing all options via the command line, you can use a configuration file in TOML, YAML, or JSON format. Use the `--config-file` or `-C` option to specify the path to your configuration file.

Command-line arguments will always take precedence over settings defined in the configuration file.

Below are examples of how to structure your configuration file.

### TOML Configuration Example (`config.toml`)

```toml
# config.toml
profiles = ["dev-profile", "prod-profile"]
regions = ["us-east-1", "eu-west-2"]
combine = true
report_name = "monthly_finops_summary"
report_type = ["csv", "pdf"] # For cost dashboard. For audit, only PDF is used.
dir = "./reports/aws-finops" # Defaults to present working directory
time_range = 30 # Defaults to 30 days
tag = ["CostCenter=Alpha", "Project=Phoenix"] # Optional
audit = false # Set to true to run audit report by default
trend = false # Set to true to run trend report by default
```

### YAML Configuration Example (`config.yaml` or `config.yml`)

```yaml
# config.yaml
profiles:
  - dev-profile
  - prod-profile
regions:
  - us-east-1
  - eu-west-2
combine: true
report_name: "monthly_finops_summary"
report_type:
  - csv
  - pdf # For cost dashboard. For audit, only PDF is used.
dir: "./reports/aws-finops"
time_range: 30
tag:
  - "CostCenter=Alpha"
  - "Project=Phoenix"
audit: false # Set to true to run audit report by default
trend: false # Set to true to run trend report by default
```

### JSON Configuration Example (`config.json`)

```json
{
  "profiles": ["dev-profile", "prod-profile"],
  "regions": ["us-east-1", "eu-west-2"],
  "combine": true,
  "report_name": "monthly_finops_summary",
  "report_type": ["csv", "pdf"], /* For cost dashboard. For audit, only PDF is used. */
  "dir": "./reports/aws-finops",
  "time_range": 30,
  "tag": ["CostCenter=Alpha", "Project=Phoenix"],
  "audit": false, /* Set to true to run audit report by default */
  "trend": false /* Set to true to run trend report by default */
}
```

## Example Terminal Output

![Dashboard](aws-finops-dashboard-v2.2.3.png)

![Trend](aws-finops-dashboard_trend.png)

![alt text](audit_report.png)

## Example PDF Reports

![alt text](audit_report_pdf.png)

![alt text](cost_report_pdf.png)

---

## Export Formats

### CSV Output Format

When exporting to CSV, a file is generated with the following columns:

- `CLI Profile`
- `AWS Account ID`
- `Last Month Cost` (or previous period based on time range)
- `Current Month Cost` (or current period based on time range)
- `Cost By Service` (Each service and its cost appears on a new line within the cell)
- `Budget Status` (Each budget's limit and actual spend appears on a new line within the cell)
- `EC2 Instances` (Each instance state and its count appears on a new line within the cell)

**Note:** Due to the multi-line formatting in some cells, it's best viewed in spreadsheet software (like Excel, Google Sheets, LibreOffice Calc) rather than plain text editors.

### JSON Output Format

When exporting to JSON, a structured file is generated that includes all dashboard data in a format that's easy to parse programmatically.

### PDF Output Format (for Audit Report)
This is the export format for reports generated using the `--audit` flag. Currently, audit reports are only exported to PDF.

When exporting to PDF, a file is generated with the following columns:

- `Profile`
- `Account ID`
- `Untagged Resources`
- `Stopped EC2 Instances`
- `Unused Volumes`
- `Unused EIPs`
- `Budget Alerts`
---

## Cost For Every Run

This script makes API calls to AWS, primarily to Cost Explorer, Budgets, EC2, and STS. AWS may charge for some API calls (typically `$0.01` for each API call, check current pricing).

The number of API calls depends heavily on the options used:

- **Cost Explorer & Budgets:** Typically 3 `ce:GetCostAndUsage` and 1 `budgets:DescribeBudgets` call per profile processed.
- **STS:** 1 `sts:GetCallerIdentity` call per profile processed (used for account ID).
- **EC2:**
  - 1 `ec2:DescribeRegions` call initially (per session).
  - If `--regions` is **not** specified, the script attempts to check accessibility by calling `ec2:DescribeInstances` in *multiple regions*, potentially increasing API calls significantly.
  - If `--regions` **is** specified, 1 `ec2:DescribeInstances` call is made *per specified region* (per profile, unless `--combine` is used, where it's called once per region for the primary profile).

**To minimize API calls and potential costs:**

- Use the `--profiles` argument to specify only the profiles you need.
- Use the `--regions` argument to limit EC2 checks to only relevant regions. This significantly reduces `ec2:DescribeInstances` calls compared to automatic region discovery.
- Consider using the `--combine` option when working with multiple profiles from the same AWS account.

The exact cost per run is usually negligible but depends on the scale of your usage and AWS pricing.

---

## Contributing

Contributions are welcome! Feel free to fork and improve the project.

### Development Setup with pip

```bash
# Fork this repository on GitHub first:
# https://github.com/ravikiranvm/aws-finops-dashboard

# Then clone your fork locally
git clone https://github.com/your-username/aws-finops-dashboard.git
cd aws-finops-dashboard

# Create and activate a virtual environment
python -m venv venv
source venv/bin/activate  # On Windows use `venv\Scripts\activate`

# Install dependencies
pip install -r requirements.txt
pip install -e ".[dev]"

# Run the formatter
hatch run fmt

# Run linters
hatch run lint

# Run the tool
python -m aws_finops_dashboard.cli --help
```

### Development Setup with uv

`uv` provides a much faster development environment setup:

```bash
# Fork this repository on GitHub first:
# https://github.com/ravikiranvm/aws-finops-dashboard

# Then clone your fork locally
git clone https://github.com/your-username/aws-finops-dashboard.git
cd aws-finops-dashboard

# Install uv if you don't have it yet
curl -LsSf https://astral.sh/uv/install.sh | sh

# Create and sync the virtual environment (.venv)
uv venv
uv pip install -e ".[dev]"

# Activate the virtual environment
source .venv/bin/activate  # On Windows use `.venv\Scripts\activate`

# Run the formatter
uv run hatch run fmt

# Run linters
uv run hatch run lint

# Run tests 
uv run hatch run test

# Run the tool
aws-finops
```

---

## Acknowledgments

Special thanks to [cschnidr](https://github.com/cschnidr) & [MKAbuMattar](https://github.com/MKAbuMattar) for their valuable contributions to significantly improve this project!

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
