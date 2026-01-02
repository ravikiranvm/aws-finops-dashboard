import os
import sys
from argparse import Namespace
from enum import Enum
from typing import Any, Dict, List, Optional, Union

import requests
import typer
from packaging import version
from rich.console import Console
from typing_extensions import Annotated

from aws_finops_dashboard.helpers import load_config_file

console = Console()

__version__ = "2.3.0"


class ReportType(str, Enum):
    """Supported report export formats."""
    csv = "csv"
    json = "json"
    pdf = "pdf"


def version_callback(value: bool) -> None:
    """Show version and exit."""
    if value:
        console.print(f"[bold red]AWS FinOps Dashboard CLI v{__version__}[/]")
        raise typer.Exit()


def welcome_banner() -> None:
    """Display the welcome banner."""
    banner = rf"""
[bold red]
  /$$$$$$  /$$      /$$  /$$$$$$        /$$$$$$$$ /$$            /$$$$$$                     
 /$$__  $$| $$  /$ | $$ /$$__  $$      | $$_____/|__/           /$$__  $$                    
| $$  \ $$| $$ /$$$| $$| $$  \__/      | $$       /$$ /$$$$$$$ | $$  \ $$  /$$$$$$   /$$$$$$$
| $$$$$$$$| $$/$$ $$ $$|  $$$$$$       | $$$$$   | $$| $$__  $$| $$  | $$ /$$__  $$ /$$_____/
| $$__  $$| $$$$_  $$$$ \____  $$      | $$__/   | $$| $$  \ $$| $$  | $$| $$  \ $$|  $$$$$$ 
| $$  | $$| $$$/ \  $$$ /$$  \ $$      | $$      | $$| $$  | $$| $$  | $$| $$  | $$ \____  $$
| $$  | $$| $$/   \  $$|  $$$$$$/      | $$      | $$| $$  | $$|  $$$$$$/| $$$$$$$/ /$$$$$$$/
|__/  |__/|__/     \__/ \______/       |__/      |__/|__/  |__/ \______/ | $$____/ |_______/ 
                                                                         | $$                
                                                                         | $$                
                                                                         |__/                
[/]
[bold bright_blue]AWS FinOps Dashboard CLI (v{__version__})[/]                                                                         
"""
    console.print(banner)


def check_latest_version() -> None:
    """Check for the latest version of the AWS FinOps Dashboard (CLI)."""
    try:
        response = requests.get(
            "https://pypi.org/pypi/aws-finops-dashboard/json", timeout=3
        )
        latest = response.json()["info"]["version"]
        if version.parse(latest) > version.parse(__version__):
            console.print(
                f"[bold red]A new version of AWS FinOps Dashboard is available: {latest}[/]"
            )
            console.print(
                "[bold bright_yellow]Please update using:\npipx upgrade aws-finops-dashboard\nor\npip install --upgrade aws-finops-dashboard\n[/]"
            )
    except Exception:
        pass


def parse_time_range(value: Optional[str]) -> Optional[Union[int, str]]:
    """Parse time range input allowing integers or the 'last-month' keyword."""
    if value is None:
        return None
    if value.lower() == "last-month":
        return "last-month"
    try:
        return int(value)
    except ValueError:
        console.print(
            "[bold red]Error: --time-range must be an integer number of days or 'last-month'[/]"
        )
        raise typer.Exit(1)


app = typer.Typer(
    name="aws-finops",
    help="A terminal-based AWS cost monitoring and optimization dashboard.",
    rich_markup_mode="rich",
    add_completion=False,
    no_args_is_help=False,
)


@app.command()
def main(
    config_file: Annotated[
        Optional[str],
        typer.Option(
            "--config-file", "-C",
            help="Path to a TOML, YAML, or JSON configuration file",
            rich_help_panel="Configuration",
        ),
    ] = None,
    
    profiles: Annotated[
        Optional[List[str]],
        typer.Option(
            "--profiles", "-p",
            help="Specific AWS profiles to use (can specify multiple)",
            rich_help_panel="Profile Selection",
        ),
    ] = None,
    all_profiles: Annotated[
        bool,
        typer.Option(
            "--all", "-a",
            help="Use all available AWS profiles from ~/.aws/config",
            rich_help_panel="Profile Selection",
        ),
    ] = False,
    combine: Annotated[
        bool,
        typer.Option(
            "--combine", "-c",
            help="Combine profiles from the same AWS account into one row",
            rich_help_panel="Profile Selection",
        ),
    ] = False,
    regions: Annotated[
        Optional[List[str]],
        typer.Option(
            "--regions", "-r",
            help="AWS regions to check for EC2 instances (can specify multiple)",
            rich_help_panel="Profile Selection",
        ),
    ] = None,
    
    time_range: Annotated[
        Optional[str],
        typer.Option(
            "--time-range", "-t",
            help="Time range: number of days (7, 30, 90) or 'last-month'",
            rich_help_panel="Time Range & Filtering",
        ),
    ] = None,
    tag: Annotated[
        Optional[List[str]],
        typer.Option(
            "--tag", "-g",
            help="Filter by cost allocation tag (e.g., --tag Team=DevOps)",
            rich_help_panel="Time Range & Filtering",
        ),
    ] = None,
    
    trend: Annotated[
        bool,
        typer.Option(
            "--trend",
            help="Show 6-month cost trend analysis with bar charts",
            rich_help_panel="Report Types",
        ),
    ] = False,
    audit: Annotated[
        bool,
        typer.Option(
            "--audit",
            help="Run FinOps audit: find untagged resources, unused volumes, stopped instances",
            rich_help_panel="Report Types",
        ),
    ] = False,
    
    report_name: Annotated[
        Optional[str],
        typer.Option(
            "--report-name", "-n",
            help="Base filename for exports (without extension)",
            rich_help_panel="Export Options",
        ),
    ] = None,
    report_type: Annotated[
        Optional[List[ReportType]],
        typer.Option(
            "--report-type", "-y",
            help="Export format: csv, json, or pdf (can specify multiple)",
            rich_help_panel="Export Options",
        ),
    ] = None,
    output_dir: Annotated[
        Optional[str],
        typer.Option(
            "--dir", "-d",
            help="Directory to save exports (default: current directory)",
            rich_help_panel="Export Options",
        ),
    ] = None,
    
    s3_bucket: Annotated[
        Optional[str],
        typer.Option(
            "--s3-bucket", "-s3",
            help="S3 bucket name for report uploads",
            rich_help_panel="S3 Export",
        ),
    ] = None,
    s3_prefix: Annotated[
        Optional[str],
        typer.Option(
            "--s3-prefix", "-s3p",
            help="S3 key prefix/folder path (e.g., reports/2025)",
            rich_help_panel="S3 Export",
        ),
    ] = None,
    s3_profile: Annotated[
        Optional[str],
        typer.Option(
            "--s3-profile", "-s3s",
            help="AWS profile to use for S3 uploads (required with --s3-bucket)",
            rich_help_panel="S3 Export",
        ),
    ] = None,
    
    slack: Annotated[
        Optional[str],
        typer.Option(
            "--slack",
            help="Send reports to Slack channel (e.g., C1234567890). Requires SLACK_BOT_TOKEN env var",
            rich_help_panel="Slack Integration",
        ),
    ] = None,
    
    show_version: Annotated[
        bool,
        typer.Option(
            "--version", "-v",
            callback=version_callback,
            is_eager=True,
            help="Show version and exit",
        ),
    ] = False,
) -> None:
    """
    [bold cyan]AWS FinOps Dashboard[/] - Monitor and optimize your AWS costs.
    
    [dim]View cost summaries, trends, and run audits across multiple AWS accounts.[/]
    
    [bold]Examples:[/]
        aws-finops                           [dim]# View costs for default profile[/]
        aws-finops -p prod dev               [dim]# View specific profiles[/]
        aws-finops --all --trend             [dim]# 6-month trend for all profiles[/]
        aws-finops --audit -r us-east-1      [dim]# Run audit in a specific region[/]
        aws-finops -n report -y pdf          [dim]# Export dashboard to PDF[/]
    """
    welcome_banner()
    check_latest_version()
    
    from aws_finops_dashboard.main import run_dashboard
    
    config_data: Optional[Dict[str, Any]] = None
    if config_file:
        config_data = load_config_file(config_file)
        if config_data is None:
            raise typer.Exit(1)
    
    # Build args namespace (required because run_dashboard expects Namespace object)
    args = Namespace(
        config_file=config_file,
        profiles=list(profiles) if profiles else None,
        all=all_profiles,
        combine=combine,
        regions=list(regions) if regions else None,
        time_range=parse_time_range(time_range),
        tag=list(tag) if tag else None,
        trend=trend,
        audit=audit,
        report_name=report_name,
        report_type=[rt.value for rt in report_type] if report_type else ["csv"],
        dir=output_dir,
        s3_bucket=s3_bucket,
        s3_prefix=s3_prefix,
        s3_profile=s3_profile,
        slack=slack,
    )
    
    # Override with config file values (same simple pattern as original argparse)
    if config_data:
        for key, value in config_data.items():
            # Map 'all' from config to args.all (Typer uses all_profiles param name)
            attr_name = key
            
            # Only override if CLI didn't set a non-default value
            if hasattr(args, attr_name):
                current = getattr(args, attr_name)
                # Override if None, False, or default report_type
                if current is None or current is False or (attr_name == "report_type" and current == ["csv"]):
                    # Parse time_range if it's a string from config
                    if attr_name == "time_range" and isinstance(value, str):
                        value = parse_time_range(value)
                    setattr(args, attr_name, value)
    
    # Validate S3 arguments
    if args.s3_bucket and args.report_name and not args.s3_profile:
        console.print(
            "[bold red]Error: --s3-profile is required when --s3-bucket is specified[/]"
        )
        console.print(
            "[yellow]Please specify which AWS profile to use for S3 upload[/]"
        )
        raise typer.Exit(1)
    
    # Validate Slack arguments
    if args.slack:
        slack_token = os.getenv("SLACK_BOT_TOKEN")
        if not slack_token:
            console.print(
                "[bold red]Error: SLACK_BOT_TOKEN environment variable is required when --slack is used[/]"
            )
            console.print(
                "[yellow]Please set SLACK_BOT_TOKEN environment variable with your Slack bot token[/]"
            )
            raise typer.Exit(1)
    
    # Run the dashboard
    result = run_dashboard(args)
    raise typer.Exit(0 if result == 0 else 1)


def cli_main() -> None:
    """Entry point wrapper for the CLI."""
    app()


if __name__ == "__main__":
    cli_main()
