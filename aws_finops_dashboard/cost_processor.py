import csv
import json
import os
from collections import defaultdict
from datetime import date, datetime, timedelta
from typing import List, Optional, Tuple

from boto3.session import Session
from rich.console import Console

from aws_finops_dashboard.aws_client import get_account_id
from aws_finops_dashboard.types import BudgetInfo, CostData, EC2Summary, ProfileData

console = Console()


def get_cost_data(session: Session, time_range: Optional[int] = None) -> CostData:
    """
    Get cost data for an AWS account.

    Args:
        session: The boto3 session to use
        time_range: Optional time range in days for cost data (default: current month)
    """
    ce = session.client("ce")
    budgets = session.client("budgets", region_name="us-east-1")
    today = date.today()

    if time_range:
        end_date = today
        start_date = today - timedelta(days=time_range)
        previous_period_end = start_date - timedelta(days=1)
        previous_period_start = previous_period_end - timedelta(days=time_range)

    else:
        start_date = today.replace(day=1)
        end_date = today

        # Last calendar month
        previous_period_end = start_date - timedelta(days=1)
        previous_period_start = previous_period_end.replace(day=1)

    account_id = get_account_id(session)

    try:
        this_period = ce.get_cost_and_usage(
            TimePeriod={"Start": start_date.isoformat(), "End": end_date.isoformat()},
            Granularity="MONTHLY",
            Metrics=["UnblendedCost"],
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
        )
    except Exception as e:
        console.log(f"[yellow]Error getting previous period cost: {e}[/]")
        previous_period = {
            "ResultsByTime": [{"Total": {"UnblendedCost": {"Amount": 0}}}]
        }

    try:
        current_period_cost_by_service = ce.get_cost_and_usage(
            TimePeriod={"Start": start_date.isoformat(), "End": end_date.isoformat()},
            Granularity="DAILY" if time_range else "MONTHLY",
            Metrics=["UnblendedCost"],
            GroupBy=[{"Type": "DIMENSION", "Key": "SERVICE"}],
        )
    except Exception as e:
        console.log(f"[yellow]Error getting current period cost by service: {e}[/]")
        current_period_cost_by_service = {"ResultsByTime": [{"Groups": []}]}

    # Aggregate cost by service across all days
    aggregated_service_costs = defaultdict(float)

    for result in current_period_cost_by_service.get("ResultsByTime", []):
        for group in result.get("Groups", []):
            service = group["Keys"][0]
            amount = float(group["Metrics"]["UnblendedCost"]["Amount"])
            aggregated_service_costs[service] += amount

    # Reformat into groups by service
    aggregated_groups = [
        {"Keys": [service], "Metrics": {"UnblendedCost": {"Amount": str(amount)}}}
        for service, amount in aggregated_service_costs.items()
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
        console.log(f"[yellow]Error getting budget data: {e}[/]")
        pass

    current_period_cost = 0.0
    for period in this_period.get("ResultsByTime", []):
        if "Total" in period and "UnblendedCost" in period["Total"]:
            current_period_cost += float(period["Total"]["UnblendedCost"]["Amount"])

    previous_period_cost = 0.0
    for period in previous_period.get("ResultsByTime", []):
        if "Total" in period and "UnblendedCost" in period["Total"]:
            previous_period_cost += float(period["Total"]["UnblendedCost"]["Amount"])

    current_period_name = (
        f"Current {time_range} days cost" if time_range else "Current month's cost"
    )
    previous_period_name = (
        f"Previous {time_range} days cost" if time_range else "Last month's cost"
    )

    return {
        "account_id": account_id,
        "current_month": current_period_cost,
        "last_month": previous_period_cost,
        "current_month_cost_by_service": aggregated_groups,
        "budgets": budgets_data,
        "current_period_name": current_period_name,
        "previous_period_name": previous_period_name,
        "time_range": time_range,
        "current_period_start": start_date.isoformat(),
        "current_period_end": end_date.isoformat(),
        "previous_period_start": previous_period_start.isoformat(),
        "previous_period_end": previous_period_end.isoformat(),
    }


def process_service_costs(
    cost_data: CostData,
) -> Tuple[List[str], List[Tuple[str, float]]]:
    """Process and format service costs from cost data."""
    service_costs: List[str] = []
    service_cost_data: List[Tuple[str, float]] = []

    for group in cost_data["current_month_cost_by_service"]:
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


def export_to_csv(
    data: List[ProfileData],
    filename: str,
    output_dir: Optional[str] = None,
    previous_period_dates: str = "N/A",
    current_period_dates: str = "N/A",
) -> Optional[str]:
    """Export dashboard data to a CSV file."""
    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.csv"

        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
            output_filename = os.path.join(output_dir, base_filename)
        else:
            output_filename = base_filename

        previous_period_header = f"Cost for period\n({previous_period_dates})"
        current_period_header = f"Cost for period\n({current_period_dates})"

        with open(output_filename, "w", newline="") as csvfile:
            fieldnames = [
                "CLI Profile",
                "AWS Account ID",
                previous_period_header,
                current_period_header,
                "Current Month Cost",
                "Cost By Service",
                "Budget Status",
                "EC2 Instances",
            ]
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()
            for row in data:

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
                        "Cost By Service": services_data or "No costs",
                        "Budget Status": budgets_data or "No budgets",
                        "EC2 Instances": ec2_data_summary or "No instances",
                    }
                )
        console.print(
            f"[bright_green]Exported dashboard data to {os.path.abspath(output_filename)}[/]"
        )
        return os.path.abspath(output_filename)
    except Exception as e:
        console.print(f"[bold red]Error exporting to CSV: {str(e)}[/]")
        return None


def export_to_json(
    data: List[ProfileData], filename: str, output_dir: Optional[str] = None
) -> Optional[str]:
    """Export dashboard data to a JSON file."""
    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.json"

        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
            output_filename = os.path.join(output_dir, base_filename)
        else:
            output_filename = base_filename

        with open(output_filename, "w") as jsonfile:
            json.dump(data, jsonfile, indent=4)

        console.print(
            f"[bright_green]Exported dashboard data to {os.path.abspath(output_filename)}[/]"
        )
        return os.path.abspath(output_filename)
    except Exception as e:
        console.print(f"[bold red]Error exporting to JSON: {str(e)}[/]")
        return None
