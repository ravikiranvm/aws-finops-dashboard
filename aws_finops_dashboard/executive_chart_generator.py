"""Matplotlib chart generation for executive PDF reports."""

import os
import tempfile
from dataclasses import dataclass
from typing import List, Sequence

from aws_finops_dashboard.report_models import (
    ChartAsset,
    CostBreakdownItem,
    DashboardProfileReport,
    DashboardReport,
)


@dataclass(frozen=True)
class GeneratedChartBundle:
    """Chart output bundle returned to the export flow."""

    temp_dir: str
    chart_assets: List[ChartAsset]


def generate_executive_chart_bundle(report: DashboardReport) -> GeneratedChartBundle:
    """Generate chart images for the executive report and return their paths."""
    temp_dir = tempfile.mkdtemp(prefix="aws_finops_exec_charts_")
    chart_assets: List[ChartAsset] = []

    try:
        import matplotlib

        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except ImportError:
        return GeneratedChartBundle(
            temp_dir=temp_dir,
            chart_assets=[
                ChartAsset(
                    title="Total Cost Trend",
                    note="matplotlib is not installed, so charts were skipped.",
                ),
                ChartAsset(
                    title="Cost Breakdown by Account/Project",
                    note="matplotlib is not installed, so charts were skipped.",
                ),
                ChartAsset(
                    title="Top Services by Cost",
                    note="matplotlib is not installed, so charts were skipped.",
                ),
            ],
        )

    chart_assets.append(_generate_total_cost_trend_chart(report, temp_dir, plt))
    chart_assets.append(_generate_account_grouped_chart(report.profiles, temp_dir, plt))
    chart_assets.append(_generate_top_services_chart(report.top_services, temp_dir, plt))
    return GeneratedChartBundle(temp_dir=temp_dir, chart_assets=chart_assets)


def _generate_total_cost_trend_chart(
    report: DashboardReport,
    temp_dir: str,
    plt,
) -> ChartAsset:
    previous_cost = report.executive_summary.previous_period_cost
    current_cost = report.executive_summary.total_cost
    if abs(previous_cost) < 0.01 and abs(current_cost) < 0.01:
        return ChartAsset(
            title="Total Cost Trend",
            note="No total cost data was available for the selected reporting periods.",
        )

    figure, axis = plt.subplots(figsize=(8, 4.5))
    labels = [report.previous_period.label, report.current_period.label]
    values = [previous_cost, current_cost]
    axis.plot(labels, values, color="#2E6E8E", linewidth=2.5, marker="o")
    axis.fill_between(labels, values, color="#DDEBF3", alpha=0.8)
    axis.set_title("Total Cost Trend", fontsize=14, fontweight="bold")
    axis.set_ylabel("Cost (USD)")
    axis.grid(axis="y", alpha=0.25)
    _apply_currency_ticks(axis, plt)
    return _save_chart(
        figure,
        temp_dir,
        "total_cost_trend.png",
        "Total Cost Trend",
    )


def _generate_account_grouped_chart(
    profiles: Sequence[DashboardProfileReport],
    temp_dir: str,
    plt,
) -> ChartAsset:
    if not profiles:
        return ChartAsset(
            title="Cost Breakdown by Account/Project",
            note="No account or project data was available for grouped cost comparison.",
        )

    labels = [profile.profile for profile in profiles]
    previous = [profile.previous_period_cost for profile in profiles]
    current = [profile.current_period_cost for profile in profiles]
    if not any(previous) and not any(current):
        return ChartAsset(
            title="Cost Breakdown by Account/Project",
            note="All grouped account or project costs were zero, so the chart was skipped.",
        )

    figure, axis = plt.subplots(figsize=(max(8, len(labels) * 1.2), 4.8))
    positions = list(range(len(labels)))
    width = 0.36
    axis.bar(
        [position - width / 2 for position in positions],
        previous,
        width=width,
        color="#9AB9CF",
        label="Previous",
    )
    axis.bar(
        [position + width / 2 for position in positions],
        current,
        width=width,
        color="#2E6E8E",
        label="Current",
    )
    axis.set_title(
        "Cost Breakdown by Account/Project",
        fontsize=14,
        fontweight="bold",
    )
    axis.set_ylabel("Cost (USD)")
    axis.set_xticks(positions)
    axis.set_xticklabels(labels, rotation=20, ha="right")
    axis.legend(frameon=False)
    axis.grid(axis="y", alpha=0.25)
    _apply_currency_ticks(axis, plt)
    return _save_chart(
        figure,
        temp_dir,
        "account_breakdown.png",
        "Cost Breakdown by Account/Project",
    )


def _generate_top_services_chart(
    top_services: Sequence[CostBreakdownItem],
    temp_dir: str,
    plt,
) -> ChartAsset:
    if not top_services:
        return ChartAsset(
            title="Top Services by Cost",
            note="No service cost data was available for the selected reporting period.",
        )

    labels = [item.name for item in top_services[:10]]
    values = [item.amount for item in top_services[:10]]
    if not any(values):
        return ChartAsset(
            title="Top Services by Cost",
            note="All top service costs were zero, so the chart was skipped.",
        )

    figure, axis = plt.subplots(figsize=(8.4, max(4.5, len(labels) * 0.45)))
    axis.barh(labels, values, color="#2E6E8E")
    axis.invert_yaxis()
    axis.set_title("Top Services by Cost", fontsize=14, fontweight="bold")
    axis.set_xlabel("Cost (USD)")
    axis.grid(axis="x", alpha=0.25)
    _apply_currency_ticks(axis, plt, axis_name="x")
    return _save_chart(
        figure,
        temp_dir,
        "top_services.png",
        "Top Services by Cost",
    )


def _save_chart(figure, temp_dir: str, file_name: str, title: str) -> ChartAsset:
    output_path = os.path.join(temp_dir, file_name)
    figure.tight_layout()
    figure.savefig(output_path, dpi=180, bbox_inches="tight")
    figure.canvas.draw()
    figure.clear()
    return ChartAsset(title=title, path=output_path)


def _apply_currency_ticks(axis, plt, axis_name: str = "y") -> None:
    formatter = plt.FuncFormatter(lambda value, _: f"${value:,.0f}")
    if axis_name == "x":
        axis.xaxis.set_major_formatter(formatter)
    else:
        axis.yaxis.set_major_formatter(formatter)
