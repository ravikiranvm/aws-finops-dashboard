"""Lightweight business-friendly insights generation for executive reports."""

import re
from typing import List, Optional, Sequence, Tuple

from aws_finops_dashboard.report_models import (
    CostBreakdownItem,
    DashboardProfileReport,
    KubernetesCostData,
    ReportInsight,
)

DEFAULT_BUDGET_THRESHOLD = 0.9


def generate_report_insights(
    profiles: Sequence[DashboardProfileReport],
    top_services: Sequence[CostBreakdownItem],
    kubernetes_costs: Optional[KubernetesCostData],
    budget_threshold: float = DEFAULT_BUDGET_THRESHOLD,
) -> List[ReportInsight]:
    """Generate concise structured insights from normalized report data."""
    insights: List[ReportInsight] = []

    increase = _biggest_change(profiles, increasing=True)
    if increase is not None:
        profile, delta = increase
        insights.append(
            ReportInsight(
                kind="biggest_increase",
                severity="medium",
                title="Largest Increase",
                message=f"{profile.profile} increased by ${delta:,.2f} versus the prior period.",
            )
        )

    decrease = _biggest_change(profiles, increasing=False)
    if decrease is not None:
        profile, delta = decrease
        insights.append(
            ReportInsight(
                kind="biggest_decrease",
                severity="low",
                title="Largest Decrease",
                message=f"{profile.profile} decreased by ${abs(delta):,.2f} versus the prior period.",
            )
        )

    if top_services:
        top_service = top_services[0]
        insights.append(
            ReportInsight(
                kind="top_service",
                severity="info",
                title="Top Service",
                message=f"{top_service.name} is the top service at ${top_service.amount:,.2f}.",
            )
        )

    if kubernetes_costs and kubernetes_costs.namespace_costs:
        top_namespace = kubernetes_costs.namespace_costs[0]
        insights.append(
            ReportInsight(
                kind="top_kubernetes_namespace",
                severity="info",
                title="Top Kubernetes Namespace",
                message=f"{top_namespace.name} is the highest-cost namespace at ${top_namespace.cost:,.2f}.",
            )
        )

    budget_insight = _budget_warning(profiles, threshold=budget_threshold)
    if budget_insight is not None:
        insights.append(budget_insight)

    no_budget_insight = _missing_budget_warning(profiles)
    if no_budget_insight is not None:
        insights.append(no_budget_insight)

    return insights


def _biggest_change(
    profiles: Sequence[DashboardProfileReport],
    increasing: bool,
) -> Optional[Tuple[DashboardProfileReport, float]]:
    deltas = [
        (profile, profile.current_period_cost - profile.previous_period_cost)
        for profile in profiles
    ]
    if increasing:
        positive = [item for item in deltas if item[1] > 0.01]
        return max(positive, key=lambda item: item[1]) if positive else None
    negative = [item for item in deltas if item[1] < -0.01]
    return min(negative, key=lambda item: item[1]) if negative else None


def _budget_warning(
    profiles: Sequence[DashboardProfileReport],
    threshold: float,
) -> Optional[ReportInsight]:
    warnings: List[Tuple[str, float, float]] = []
    for profile in profiles:
        budgets = _parse_budget_items(profile.budget_items)
        for name, values in budgets.items():
            actual = values.get("actual")
            limit = values.get("limit")
            if actual is None or limit is None or limit <= 0:
                continue
            if actual > (limit * threshold):
                warnings.append((profile.profile, name, actual / limit))
    if not warnings:
        return None
    profile_name, budget_name, ratio = max(warnings, key=lambda item: item[2])
    return ReportInsight(
        kind="budget_warning",
        severity="high",
        title="Budget Attention",
        message=f"{profile_name} is above {threshold:.0%} of budget on {budget_name}.",
    )


def _missing_budget_warning(
    profiles: Sequence[DashboardProfileReport],
) -> Optional[ReportInsight]:
    missing = [
        profile.profile
        for profile in profiles
        if any("no budgets found" in item.lower() for item in profile.budget_items)
    ]
    if not missing:
        return None
    return ReportInsight(
        kind="no_budget_warning",
        severity="medium",
        title="Missing Budget",
        message=f"Budget coverage is missing for {', '.join(missing[:3])}.",
    )


def _parse_budget_items(items: Sequence[str]) -> dict:
    parsed = {}
    for item in items:
        match = re.match(
            r"^(?P<name>.+?)\s+(?P<field>limit|actual|forecast):\s+\$(?P<value>[0-9,]+(?:\.[0-9]+)?)$",
            item.strip(),
            re.IGNORECASE,
        )
        if not match:
            continue
        name = match.group("name")
        field = match.group("field").lower()
        value = float(match.group("value").replace(",", ""))
        parsed.setdefault(name, {})[field] = value
    return parsed
