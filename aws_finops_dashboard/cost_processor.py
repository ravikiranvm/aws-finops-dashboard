import csv
import json
import os
from collections import defaultdict
from datetime import date, datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple, Union
from io import StringIO

from boto3.session import Session
from rich.console import Console

from aws_finops_dashboard.aws_client import get_account_id
from aws_finops_dashboard.types import BudgetInfo, CostData, EC2Summary, ProfileData

console = Console()


def get_trend(session: Session, tag: Optional[List[str]] = None) -> Dict[str, Any]:
    """
    Get cost trend data for an AWS account.

    Args:
        session: The boto3 session to use
        tag: Optional list of tags in "Key=Value" format to filter resources.

    """
    ce = session.client("ce")
    tag_filters: List[Dict[str, Any]] = []
    if tag:
        for t in tag:
            key, value = t.split("=", 1)
            tag_filters.append({"Key": key, "Values": [value]})

    filter_param: Optional[Dict[str, Any]] = None
    if tag_filters:
        if len(tag_filters) == 1:
            filter_param = {
                "Tags": {
                    "Key": tag_filters[0]["Key"],
                    "Values": tag_filters[0]["Values"],
                    "MatchOptions": ["EQUALS"],
                }
            }

        else:
            filter_param = {
                "And": [
                    {
                        "Tags": {
                            "Key": f["Key"],
                            "Values": f["Values"],
                            "MatchOptions": ["EQUALS"],
                        }
                    }
                    for f in tag_filters
                ]
            }
    kwargs = {}
    if filter_param:
        kwargs["Filter"] = filter_param

    end_date = date.today()
    start_date = (end_date - timedelta(days=180)).replace(day=1)
    account_id = get_account_id(session)
    profile = session.profile_name

    monthly_costs = []

    try:
        monthly_data = ce.get_cost_and_usage(
            TimePeriod={
                "Start": start_date.isoformat(),
                "End": end_date.isoformat(),
            },
            Granularity="MONTHLY",
            Metrics=["UnblendedCost"],
            **kwargs,
        )
        for period in monthly_data.get("ResultsByTime", []):
            month = datetime.strptime(
                period["TimePeriod"]["Start"], "%Y-%m-%d"
            ).strftime("%b %Y")
            cost = float(period["Total"]["UnblendedCost"]["Amount"])
            monthly_costs.append((month, cost))
    except Exception as e:
        console.log(f"[yellow]Error getting monthly trend data: {e}[/]")
        monthly_costs = []

    return {
        "monthly_costs": monthly_costs,
        "account_id": account_id,
        "profile": profile,
    }


def get_cost_data(
    session: Session,
    time_range: Optional[Union[int, str]] = None,
    tag: Optional[List[str]] = None,
    get_trend: bool = False,
) -> CostData:
    """
    Get cost data for an AWS account.

    Args:
        session: The boto3 session to use
        time_range: Optional time range in days for cost data (default: current month)
        tag: Optional list of tags in "Key=Value" format to filter resources.
        get_trend: Optional boolean to get trend data for last 6 months (default).

    """
    ce = session.client("ce")
    budgets = session.client("budgets", region_name="us-east-1")
    today = date.today()

    tag_filters: List[Dict[str, Any]] = []
    if tag:
        for t in tag:
            key, value = t.split("=", 1)
            tag_filters.append({"Key": key, "Values": [value]})

    filter_param: Optional[Dict[str, Any]] = None
    if tag_filters:
        if len(tag_filters) == 1:
            filter_param = {
                "Tags": {
                    "Key": tag_filters[0]["Key"],
                    "Values": tag_filters[0]["Values"],
                    "MatchOptions": ["EQUALS"],
                }
            }

        else:
            filter_param = {
                "And": [
                    {
                        "Tags": {
                            "Key": f["Key"],
                            "Values": f["Values"],
                            "MatchOptions": ["EQUALS"],
                        }
                    }
                    for f in tag_filters
                ]
            }
    kwargs = {}
    if filter_param:
        kwargs["Filter"] = filter_param

    if time_range == "last-month":
        # Current period is the previous calendar month
        end_date = today.replace(day=1)
        start_date = (end_date - timedelta(days=1)).replace(day=1)

        # Previous period is the month before last
        previous_period_end = start_date
        previous_period_start = (start_date - timedelta(days=1)).replace(day=1)

    elif time_range:
        end_date = today
        start_date = today - timedelta(days=time_range)
        previous_period_end = start_date
        previous_period_start = (start_date - timedelta(days=time_range))

    else:
        start_date = today.replace(day=1)
        end_date = today

        # Edge case when user runs the tool on the first day of the month
        if start_date == end_date:
            end_date += timedelta(days=1)

        # Last calendar month
        previous_period_end = start_date
        previous_period_start = (start_date - timedelta(days=1)).replace(day=1)

    account_id = get_account_id(session)

    try:
        this_period = ce.get_cost_and_usage(
            TimePeriod={"Start": start_date.isoformat(), "End": end_date.isoformat()},
            Granularity="MONTHLY",
            Metrics=["UnblendedCost"],
            **kwargs,
        )
    except Exception as e:
        console.log(f"[yellow]Error getting current period cost: {e}[/]")
        this_period = {"ResultsByTime": [{"Total": {"UnblendedCost": {"Amount": 0}}}]}

    try:
        previous_period = ce.get_cost_and_usage(
            TimePeriod={
                "Start": previous_period_start.isoformat(),
                "End": previous_period_end.isoformat(),
            },
            Granularity="MONTHLY",
            Metrics=["UnblendedCost"],
            **kwargs,
        )
    except Exception as e:
        console.log(f"[yellow]Error getting previous period cost: {e}[/]")
        previous_period = {
            "ResultsByTime": [{"Total": {"UnblendedCost": {"Amount": 0}}}]
        }

    granularity = "DAILY" if isinstance(time_range, int) and time_range else "MONTHLY"

    try:
        current_period_cost_by_service = ce.get_cost_and_usage(
            TimePeriod={"Start": start_date.isoformat(), "End": end_date.isoformat()},
            Granularity=granularity,
            Metrics=["UnblendedCost"],
            GroupBy=[{"Type": "DIMENSION", "Key": "SERVICE"}],
            **kwargs,
        )
    except Exception as e:
        console.log(f"[yellow]Error getting current period cost by service: {e}[/]")
        current_period_cost_by_service = {"ResultsByTime": [{"Groups": []}]}

    try:
        previous_period_cost_by_service = ce.get_cost_and_usage(
            TimePeriod={
                "Start": previous_period_start.isoformat(),
                "End": previous_period_end.isoformat(),
            },
            Granularity=granularity,
            Metrics=["UnblendedCost"],
            GroupBy=[{"Type": "DIMENSION", "Key": "SERVICE"}],
            **kwargs,
        )
    except Exception as e:
        console.log(f"[yellow]Error getting previous period cost by service: {e}[/]")
        previous_period_cost_by_service = {"ResultsByTime": [{"Groups": []}]}

    # Aggregate cost by service across all days
    aggregated_service_costs: Dict[str, float] = defaultdict(float)
    for result in current_period_cost_by_service.get("ResultsByTime", []):
        for group in result.get("Groups", []):
            service = group["Keys"][0]
            amount = float(group["Metrics"]["UnblendedCost"]["Amount"])
            aggregated_service_costs[service] += amount

    aggregated_previous_service_costs: Dict[str, float] = defaultdict(float)
    for result in previous_period_cost_by_service.get("ResultsByTime", []):
        for group in result.get("Groups", []):
            service = group["Keys"][0]
            amount = float(group["Metrics"]["UnblendedCost"]["Amount"])
            aggregated_previous_service_costs[service] += amount

    # Reformat into groups by service
    aggregated_groups = [
        {"Keys": [service], "Metrics": {"UnblendedCost": {"Amount": str(amount)}}}
        for service, amount in aggregated_service_costs.items()
    ]
    aggregated_previous_groups = [
        {"Keys": [service], "Metrics": {"UnblendedCost": {"Amount": str(amount)}}}
        for service, amount in aggregated_previous_service_costs.items()
    ]

    budgets_data: List[BudgetInfo] = []
    try:
        response = budgets.describe_budgets(AccountId=account_id)
        for budget in response["Budgets"]:
            budgets_data.append(
                {
                    "name": budget["BudgetName"],
                    "limit": float(budget["BudgetLimit"]["Amount"]),
                    "actual": float(budget["CalculatedSpend"]["ActualSpend"]["Amount"]),
                    "forecast": float(
                        budget["CalculatedSpend"]
                        .get("ForecastedSpend", {})
                        .get("Amount", 0.0)
                    )
                    or None,
                }
            )
    except Exception as e:
        pass

    current_period_cost = 0.0
    for period in this_period.get("ResultsByTime", []):
        if "Total" in period and "UnblendedCost" in period["Total"]:
            current_period_cost += float(period["Total"]["UnblendedCost"]["Amount"])

    previous_period_cost = 0.0
    for period in previous_period.get("ResultsByTime", []):
        if "Total" in period and "UnblendedCost" in period["Total"]:
            previous_period_cost += float(period["Total"]["UnblendedCost"]["Amount"])

    if time_range == "last-month":
        current_period_name = "Last month's cost"
        previous_period_name = "Prior month's cost"
    elif time_range:
        current_period_name = f"Current {time_range} days cost"
        previous_period_name = f"Previous {time_range} days cost"
    else:
        current_period_name = "Current month's cost"
        previous_period_name = "Last month's cost"

    return {
        "account_id": account_id,
        "current_month": current_period_cost,
        "last_month": previous_period_cost,
        "current_month_cost_by_service": aggregated_groups,
        "previous_month_cost_by_service": aggregated_previous_groups,
        "budgets": budgets_data,
        "current_period_name": current_period_name,
        "previous_period_name": previous_period_name,
        "time_range": time_range,
        "current_period_start": start_date.isoformat(),
        "current_period_end": (end_date - timedelta(days=1)).isoformat(),
        "previous_period_start": previous_period_start.isoformat(),
        "previous_period_end": (previous_period_end - timedelta(days=1)).isoformat(),
        "monthly_costs": None,
    }


def process_service_costs(
    groups: List[Dict[str, Any]],
) -> Tuple[List[str], List[Tuple[str, float]]]:
    """Process and format service costs from Cost Explorer groups."""
    service_costs: List[str] = []
    service_cost_data: List[Tuple[str, float]] = []

    for group in groups:
        if "Keys" in group and "Metrics" in group:
            service_name = group["Keys"][0]
            cost_amount = float(group["Metrics"]["UnblendedCost"]["Amount"])
            if cost_amount > 0.001:
                service_cost_data.append((service_name, cost_amount))

    service_cost_data.sort(key=lambda x: x[1], reverse=True)

    if not service_cost_data:
        service_costs.append("No costs associated with this account")
    else:
        for service_name, cost_amount in service_cost_data:
            service_costs.append(f"{service_name}: ${cost_amount:.2f}")

    return service_costs, service_cost_data


def format_budget_info(budgets: List[BudgetInfo]) -> List[str]:
    """Format budget information for display."""
    budget_info: List[str] = []
    for budget in budgets:
        budget_info.append(f"{budget['name']} limit: ${budget['limit']}")
        budget_info.append(f"{budget['name']} actual: ${budget['actual']:.2f}")
        if budget["forecast"] is not None:
            budget_info.append(f"{budget['name']} forecast: ${budget['forecast']:.2f}")

    if not budget_info:
        budget_info.append("No budgets found;\nCreate a budget for this account")

    return budget_info


def format_ec2_summary(ec2_data: EC2Summary) -> List[str]:
    """Format EC2 instance summary for display."""
    ec2_summary_text: List[str] = []
    for state, count in sorted(ec2_data.items()):
        if count > 0:
            state_color = (
                "bright_green"
                if state == "running"
                else "bright_yellow" if state == "stopped" else "bright_cyan"
            )
            ec2_summary_text.append(f"[{state_color}]{state}: {count}[/]")

    if not ec2_summary_text:
        ec2_summary_text = ["No instances found"]

    return ec2_summary_text


def change_in_total_cost(
    current_period: float, previous_period: float
) -> Optional[float]:
    """Calculate the  change in total cost between current period and previous period."""
    if abs(previous_period) < 0.01:
        if abs(current_period) < 0.01:
            return 0.00  # No change if both periods are zero
        return None  # Undefined percentage change if previous is zero but current is non-zero

    return ((current_period - previous_period) / previous_period) * 100.00


def export_to_csv(
    data: List[ProfileData],
    filename: str,
    output_dir: Optional[str] = None,
    previous_period_dates: str = "N/A",
    current_period_dates: str = "N/A",
    export_handler=None,
) -> Optional[str]:
    """Export dashboard data to a CSV file or S3."""
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.csv"

        csv_buffer = StringIO()

        previous_period_header = f"Cost for period\n({previous_period_dates})"
        current_period_header = f"Cost for period\n({current_period_dates})"

        fieldnames = [
            "CLI Profile",
            "AWS Account ID",
            previous_period_header,
            current_period_header,
            "Previous Period Cost By Service",
            "Current Period Cost By Service",
            "Budget Status",
            "EC2 Instances",
        ]
        writer = csv.DictWriter(csv_buffer, fieldnames=fieldnames)
        writer.writeheader()
        for row in data:
            prev_services_data = "\n".join(
                [
                    f"{service}: ${cost:.2f}"
                    for service, cost in row["previous_service_costs"]
                ]
            )
            services_data = "\n".join(
                [
                    f"{service}: ${cost:.2f}"
                    for service, cost in row["service_costs"]
                ]
            )

            budgets_data = (
                "\n".join(row["budget_info"])
                if row["budget_info"]
                else "No budgets"
            )

            ec2_data_summary = "\n".join(
                [
                    f"{state}: {count}"
                    for state, count in row["ec2_summary"].items()
                    if count > 0
                ]
            )

            writer.writerow(
                {
                    "CLI Profile": row["profile"],
                    "AWS Account ID": row["account_id"],
                    previous_period_header: f"${row['last_month']:.2f}",
                    current_period_header: f"${row['current_month']:.2f}",
                    "Previous Period Cost By Service": prev_services_data or "No costs",
                    "Current Period Cost By Service": services_data or "No costs",
                    "Budget Status": budgets_data or "No budgets",
                    "EC2 Instances": ec2_data_summary or "No instances",
                }
            )

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=output_dir)

        csv_content = csv_buffer.getvalue().encode("utf-8")
        saved_path = export_handler.save(csv_content, base_filename, "text/csv")

        return saved_path

    except Exception as e:
        console.print(f"[bold red]Error exporting to CSV: {str(e)}[/]")
        return None


def export_to_json(
    data: List[ProfileData],
    filename: str,
    output_dir: Optional[str] = None,
    export_handler=None,
) -> Optional[str]:
    """Export dashboard data to a JSON file or S3."""
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.json"

        json_content = json.dumps(data, indent=4).encode("utf-8")

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=output_dir)

        saved_path = export_handler.save(json_content, base_filename, "application/json")

        return saved_path

    except Exception as e:
        console.print(f"[bold red]Error exporting to JSON: {str(e)}[/]")
        return None
