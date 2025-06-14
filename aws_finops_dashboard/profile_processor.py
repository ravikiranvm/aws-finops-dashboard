import os
from collections import defaultdict
from typing import Dict, List, Optional

import boto3
from rich.console import Console

from aws_finops_dashboard.aws_client import (
    ec2_summary,
    get_accessible_regions,
)
from aws_finops_dashboard.cost_processor import (
    change_in_total_cost,
    format_budget_info,
    format_ec2_summary,
    get_cost_data,
    process_service_costs,
)
from aws_finops_dashboard.types import (
    BudgetInfo,
    CostData,
    ProfileData,
)

console = Console()


def process_single_profile(
    profile: str,
    user_regions: Optional[List[str]] = None,
    time_range: Optional[int] = None,
    tag: Optional[List[str]] = None,
) -> ProfileData:
    """Process a single AWS profile and return its data."""
    try:
        session: boto3.Session
        if (
            os.getenv("AWS_ACCESS_KEY_ID")
            and os.getenv("AWS_SECRET_ACCESS_KEY")
        ):
            console.log(
                "Using AWS credentials from environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN if set)."
            )
            session = boto3.Session()
        elif profile:
            console.log(f"Using AWS profile: {profile}")
            session = boto3.Session(profile_name=profile)
        else:
            console.log("No specific profile provided and no overriding environment credentials. Attempting default AWS session resolution.")
            session = boto3.Session()

        cost_data = get_cost_data(session, time_range, tag)

        if user_regions:
            profile_regions = user_regions
        else:
            profile_regions = get_accessible_regions(session)

        ec2_data = ec2_summary(session, profile_regions)
        service_costs, service_cost_data = process_service_costs(cost_data)
        budget_info = format_budget_info(cost_data["budgets"])
        account_id = cost_data.get("account_id", "Unknown") or "Unknown"
        ec2_summary_text = format_ec2_summary(ec2_data)
        percent_change_in_total_cost = change_in_total_cost(
            cost_data["current_month"], cost_data["last_month"]
        )

        return {
            "profile": profile,
            "account_id": account_id,
            "last_month": cost_data["last_month"],
            "current_month": cost_data["current_month"],
            "service_costs": service_cost_data,
            "service_costs_formatted": service_costs,
            "budget_info": budget_info,
            "ec2_summary": ec2_data,
            "ec2_summary_formatted": ec2_summary_text,
            "success": True,
            "error": None,
            "current_period_name": cost_data["current_period_name"],
            "previous_period_name": cost_data["previous_period_name"],
            "percent_change_in_total_cost": percent_change_in_total_cost,
        }

    except Exception as e:
        return {
            "profile": profile,
            "account_id": "Error",
            "last_month": 0,
            "current_month": 0,
            "service_costs": [],
            "service_costs_formatted": [f"Failed to process profile: {str(e)}"],
            "budget_info": ["N/A"],
            "ec2_summary": {"N/A": 0},
            "ec2_summary_formatted": ["Error"],
            "success": False,
            "error": str(e),
            "current_period_name": "Current month",
            "previous_period_name": "Last month",
            "percent_change_in_total_cost": None,
        }


def process_combined_profiles(
    account_id: str,
    profiles: List[str],
    user_regions: Optional[List[str]] = None,
    time_range: Optional[int] = None,
    tag: Optional[List[str]] = None,
) -> ProfileData:
    """Process multiple profiles from the same AWS account."""

    primary_profile = profiles[0]

    primary_session: boto3.Session
    if (
        os.getenv("AWS_ACCESS_KEY_ID")
        and os.getenv("AWS_SECRET_ACCESS_KEY")
    ):
        console.log(
            "Using AWS credentials from environment variables for combined profile processing (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN if set)."
        )
        primary_session = boto3.Session()
    elif primary_profile:
        console.log(f"Using AWS profile for combined processing: {primary_profile}")
        primary_session = boto3.Session(profile_name=primary_profile)
    else:
        console.log("No specific primary profile for combined processing and no overriding environment credentials. Attempting default AWS session resolution.")
        primary_session = boto3.Session()

    account_cost_data: CostData = {
        "account_id": account_id,
        "current_month": 0.0,
        "last_month": 0.0,
        "current_month_cost_by_service": [],
        "budgets": [],
        "current_period_name": "Current month",
        "previous_period_name": "Last month",
        "time_range": time_range,
        "current_period_start": "N/A",
        "current_period_end": "N/A",
        "previous_period_start": "N/A",
        "previous_period_end": "N/A",
        "monthly_costs": None,
    }

    try:
        # Attempt to overwrite with actual data from Cost Explorer
        account_cost_data = get_cost_data(primary_session, time_range, tag)
    except Exception as e:
        console.log(
            f"[bold red]Error getting cost data for account {account_id}: {str(e)}[/]"
        )
        # account_cost_data retains its default values if an error occurs

    combined_current_month = account_cost_data["current_month"]
    combined_last_month = account_cost_data["last_month"]
    combined_service_costs_dict: Dict[str, float] = defaultdict(float)

    for group in account_cost_data["current_month_cost_by_service"]:
        if "Keys" in group and "Metrics" in group:
            service_name = group["Keys"][0]
            cost_amount = float(group["Metrics"]["UnblendedCost"]["Amount"])
            if cost_amount > 0.001:
                combined_service_costs_dict[service_name] += cost_amount

    combined_budgets = account_cost_data["budgets"]

    if user_regions:
        primary_regions = user_regions
    else:
        primary_regions = get_accessible_regions(primary_session)

    combined_ec2 = ec2_summary(primary_session, primary_regions)

    service_costs = []
    service_cost_data = [
        (service, cost)
        for service, cost in combined_service_costs_dict.items()
        if cost > 0.001
    ]
    service_cost_data.sort(key=lambda x: x[1], reverse=True)

    if not service_cost_data:
        service_costs.append("No costs associated with this account")
    else:
        for service_name, cost_amount in service_cost_data:
            service_costs.append(f"{service_name}: ${cost_amount:.2f}")

    budget_info = format_budget_info(combined_budgets)

    ec2_summary_text = format_ec2_summary(combined_ec2)

    profile_list = ", ".join(profiles)

    percent_change_in_total_cost = change_in_total_cost(
        combined_current_month, combined_last_month
    )

    return {
        "profile": profile_list,
        "account_id": account_id,
        "last_month": combined_last_month,
        "current_month": combined_current_month,
        "service_costs": service_cost_data,
        "service_costs_formatted": service_costs,
        "budget_info": budget_info,
        "ec2_summary": combined_ec2,
        "ec2_summary_formatted": ec2_summary_text,
        "success": True,
        "error": None,
        "current_period_name": account_cost_data["current_period_name"],
        "previous_period_name": account_cost_data["previous_period_name"],
        "percent_change_in_total_cost": percent_change_in_total_cost,
    }
