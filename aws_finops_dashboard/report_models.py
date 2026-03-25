"""Domain models for report exports and PDF rendering."""

from dataclasses import dataclass
from datetime import datetime
from typing import Dict, List, Optional, Sequence, Tuple

from aws_finops_dashboard.types import ProfileData


@dataclass(frozen=True)
class ReportPeriod:
    """Represents a labeled reporting period."""

    label: str
    date_range: str


@dataclass(frozen=True)
class CostBreakdownItem:
    """Represents a named cost entry."""

    name: str
    amount: float


@dataclass(frozen=True)
class ChartAsset:
    """Represents a chart image available for embedding in a PDF."""

    title: str
    path: Optional[str] = None
    note: Optional[str] = None


@dataclass(frozen=True)
class ReportSection:
    """Represents a titled list section in a report."""

    title: str
    items: List[str]


@dataclass(frozen=True)
class ExecutiveReportOptions:
    """Optional executive report presentation inputs."""

    logo_path: Optional[str] = None
    confidentiality_notice: Optional[str] = None
    chart_assets: Optional[List[ChartAsset]] = None


@dataclass(frozen=True)
class KubernetesCostItem:
    """Represents a Kubernetes cost grouped by a logical name."""

    name: str
    cost: float


@dataclass(frozen=True)
class KubernetesCostData:
    """Normalized Kubernetes cost data sourced from OpenCost."""

    source: str
    window: str
    total_cost: float
    namespace_costs: List[KubernetesCostItem]
    workload_costs: List[KubernetesCostItem]
    idle_cost: Optional[float] = None
    shared_cost: Optional[float] = None
    unallocated_cost: Optional[float] = None
    warnings: Optional[List[str]] = None


@dataclass(frozen=True)
class ReportInsight:
    """Structured narrative insight for summaries and reports."""

    kind: str
    severity: str
    title: str
    message: str


@dataclass(frozen=True)
class DashboardExecutiveSummary:
    """Executive summary KPIs for the dashboard report."""

    total_cost: float
    previous_period_cost: float
    delta_amount: float
    delta_percentage: Optional[float]
    top_cost_driver: str


@dataclass(frozen=True)
class DashboardProfileReport:
    """PDF-friendly dashboard report data for one profile or account."""

    profile: str
    account_id: str
    previous_period_cost: float
    current_period_cost: float
    previous_services: List[CostBreakdownItem]
    current_services: List[CostBreakdownItem]
    budget_items: List[str]
    inventory_items: List[str]
    percent_change_in_total_cost: Optional[float]


@dataclass(frozen=True)
class DashboardReport:
    """Root model for cost dashboard PDF rendering."""

    title: str
    previous_period: ReportPeriod
    current_period: ReportPeriod
    profiles: List[DashboardProfileReport]
    generated_at: datetime
    executive_summary: DashboardExecutiveSummary
    top_services: List[CostBreakdownItem]
    observations: List[str]
    insights: List[ReportInsight]
    options: ExecutiveReportOptions
    kubernetes_costs: Optional[KubernetesCostData] = None


@dataclass(frozen=True)
class AuditProfileReport:
    """PDF-friendly audit report data for one profile or account."""

    profile: str
    account_id: str
    sections: List[ReportSection]


@dataclass(frozen=True)
class AuditReport:
    """Root model for audit PDF rendering."""

    title: str
    profiles: List[AuditProfileReport]
    generated_at: datetime
    footer_note: Optional[str] = None


def build_dashboard_report(
    data: Sequence[ProfileData],
    previous_period_dates: str,
    current_period_dates: str,
    previous_period_name: str = "Previous Period",
    current_period_name: str = "Current Period",
    logo_path: Optional[str] = None,
    confidentiality_notice: Optional[str] = None,
    chart_image_paths: Optional[Sequence[str]] = None,
    chart_assets: Optional[Sequence[ChartAsset]] = None,
    kubernetes_costs: Optional[KubernetesCostData] = None,
) -> DashboardReport:
    """Adapt existing dashboard export payloads into report domain models."""
    from aws_finops_dashboard.insights_engine import generate_report_insights

    profiles = [
        DashboardProfileReport(
            profile=row["profile"],
            account_id=row["account_id"],
            previous_period_cost=row["last_month"],
            current_period_cost=row["current_month"],
            previous_services=_to_cost_items(row["previous_service_costs"]),
            current_services=_to_cost_items(row["service_costs"]),
            budget_items=row["budget_info"] or ["No budgets"],
            inventory_items=_to_inventory_items(row["ec2_summary"]),
            percent_change_in_total_cost=row.get("percent_change_in_total_cost"),
        )
        for row in data
    ]
    top_services = _aggregate_top_services(profiles)
    executive_summary = _build_executive_summary(profiles, top_services)
    insights = generate_report_insights(
        profiles=profiles,
        top_services=top_services,
        kubernetes_costs=kubernetes_costs,
    )
    return DashboardReport(
        title="AWS FinOps Dashboard (Cost Report)",
        previous_period=ReportPeriod(
            label=previous_period_name,
            date_range=previous_period_dates,
        ),
        current_period=ReportPeriod(
            label=current_period_name,
            date_range=current_period_dates,
        ),
        profiles=profiles,
        generated_at=datetime.now(),
        executive_summary=executive_summary,
        top_services=top_services,
        observations=[insight.message for insight in insights],
        insights=insights,
        options=ExecutiveReportOptions(
            logo_path=logo_path,
            confidentiality_notice=confidentiality_notice,
            chart_assets=_resolve_chart_assets(chart_assets, chart_image_paths),
        ),
        kubernetes_costs=kubernetes_costs,
    )


def build_audit_report(audit_data_list: Sequence[Dict[str, str]]) -> AuditReport:
    """Adapt existing audit export payloads into report domain models."""
    profiles = [
        AuditProfileReport(
            profile=row["profile"],
            account_id=row["account_id"],
            sections=[
                ReportSection(
                    title="Untagged Resources",
                    items=_split_to_items(row.get("untagged_resources", "")),
                ),
                ReportSection(
                    title="Stopped EC2 Instances",
                    items=_split_to_items(row.get("stopped_instances", "")),
                ),
                ReportSection(
                    title="Unused Volumes",
                    items=_split_to_items(row.get("unused_volumes", "")),
                ),
                ReportSection(
                    title="Unused EIPs",
                    items=_split_to_items(row.get("unused_eips", "")),
                ),
                ReportSection(
                    title="Budget Alerts",
                    items=_split_to_items(row.get("budget_alerts", "")),
                ),
            ],
        )
        for row in audit_data_list
    ]
    return AuditReport(
        title="AWS FinOps Dashboard (Audit Report)",
        profiles=profiles,
        generated_at=datetime.now(),
        footer_note="Note: This report lists untagged EC2, RDS, Lambda, ELBv2 only.",
    )


def _to_cost_items(items: Sequence[Tuple[str, float]]) -> List[CostBreakdownItem]:
    return [CostBreakdownItem(name=name, amount=amount) for name, amount in items]


def _to_inventory_items(ec2_summary: Dict[str, int]) -> List[str]:
    items = [
        f"{state}: {count}" for state, count in ec2_summary.items() if count > 0
    ]
    return items or ["No instances"]


def _split_to_items(value: str) -> List[str]:
    if not value:
        return ["None"]
    items = [line.strip() for line in value.splitlines() if line.strip()]
    return items or ["None"]


def _resolve_chart_assets(
    chart_assets: Optional[Sequence[ChartAsset]],
    chart_image_paths: Optional[Sequence[str]],
) -> Optional[List[ChartAsset]]:
    if chart_assets is not None:
        return list(chart_assets)
    if not chart_image_paths:
        return None
    return [
        ChartAsset(title=f"Chart {index + 1}", path=path)
        for index, path in enumerate(chart_image_paths)
    ]


def _aggregate_top_services(
    profiles: Sequence[DashboardProfileReport],
) -> List[CostBreakdownItem]:
    totals: Dict[str, float] = {}
    for profile in profiles:
        for item in profile.current_services:
            totals[item.name] = totals.get(item.name, 0.0) + item.amount
    ranked = sorted(totals.items(), key=lambda item: item[1], reverse=True)
    return [CostBreakdownItem(name=name, amount=amount) for name, amount in ranked[:10]]


def _build_executive_summary(
    profiles: Sequence[DashboardProfileReport],
    top_services: Sequence[CostBreakdownItem],
) -> DashboardExecutiveSummary:
    total_cost = sum(profile.current_period_cost for profile in profiles)
    previous_period_cost = sum(profile.previous_period_cost for profile in profiles)
    delta_amount = total_cost - previous_period_cost
    delta_percentage: Optional[float]
    if abs(previous_period_cost) < 0.01:
        delta_percentage = None if abs(total_cost) >= 0.01 else 0.0
    else:
        delta_percentage = (delta_amount / previous_period_cost) * 100.0
    top_cost_driver = (
        f"{top_services[0].name} (${top_services[0].amount:,.2f})"
        if top_services
        else "No significant cost driver"
    )
    return DashboardExecutiveSummary(
        total_cost=total_cost,
        previous_period_cost=previous_period_cost,
        delta_amount=delta_amount,
        delta_percentage=delta_percentage,
        top_cost_driver=top_cost_driver,
    )

