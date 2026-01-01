"""Type definitions for AWS FinOps Dashboard."""

from typing import Dict, List, Optional, Tuple, TypedDict, Union


class BudgetInfo(TypedDict):
    """Type for a budget entry."""

    name: str
    limit: float
    actual: float
    forecast: Optional[float]


class CostData(TypedDict):
    """Type for cost data returned from AWS Cost Explorer."""

    account_id: Optional[str]
    current_month: float
    last_month: float
    current_month_cost_by_service: List[Dict]
    previous_month_cost_by_service: List[Dict]
    budgets: List[BudgetInfo]
    current_period_name: str
    previous_period_name: str
    time_range: Optional[Union[int, str]]
    current_period_start: str
    current_period_end: str
    previous_period_start: str
    previous_period_end: str
    monthly_costs: Optional[List[Tuple[str, float]]]


class ProfileData(TypedDict):
    """Type for processed profile data."""

    profile: str
    account_id: str
    last_month: float
    current_month: float
    service_costs: List[Tuple[str, float]]
    service_costs_formatted: List[str]
    previous_service_costs: List[Tuple[str, float]]
    previous_service_costs_formatted: List[str]
    budget_info: List[str]
    ec2_summary: Dict[str, int]
    ec2_summary_formatted: List[str]
    success: bool
    error: Optional[str]
    current_period_name: str
    previous_period_name: str
    percent_change_in_total_cost: Optional[float]


class CLIArgs(TypedDict, total=False):
    """Type for CLI arguments."""

    profiles: Optional[List[str]]
    regions: Optional[List[str]]
    all: bool
    combine: bool
    report_name: Optional[str]
    report_type: Optional[List[str]]
    dir: Optional[str]
    time_range: Optional[Union[int, str]]


RegionName = str
EC2Summary = Dict[str, int]
