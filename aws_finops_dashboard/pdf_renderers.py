"""PDF renderer selection and orchestration."""

from typing import Dict, Literal, Optional, Sequence

from reportlab.platypus import SimpleDocTemplate

from aws_finops_dashboard.report_models import (
    ChartAsset,
    KubernetesCostData,
    build_audit_report,
    build_dashboard_report,
)
from aws_finops_dashboard.types import ProfileData

PdfStyle = Literal["legacy", "executive"]


def render_cost_dashboard_pdf(
    pdf_output: object,
    data: Sequence[ProfileData],
    previous_period_dates: str,
    current_period_dates: str,
    pdf_style: PdfStyle = "legacy",
    previous_period_name: str = "Previous Period",
    current_period_name: str = "Current Period",
    logo_path: Optional[str] = None,
    confidentiality_notice: Optional[str] = None,
    chart_image_paths: Optional[Sequence[str]] = None,
    chart_assets: Optional[Sequence[ChartAsset]] = None,
    kubernetes_costs: Optional[KubernetesCostData] = None,
) -> None:
    """Render the dashboard report PDF using the requested style."""
    report = build_dashboard_report(
        data,
        previous_period_dates=previous_period_dates,
        current_period_dates=current_period_dates,
        previous_period_name=previous_period_name,
        current_period_name=current_period_name,
        logo_path=logo_path,
        confidentiality_notice=confidentiality_notice,
        chart_image_paths=chart_image_paths,
        chart_assets=chart_assets,
        kubernetes_costs=kubernetes_costs,
    )
    doc = _build_doc_template(pdf_output)
    if pdf_style == "executive":
        from aws_finops_dashboard.executive_pdf_renderer import (
            render_dashboard_report as render_dashboard_report_impl,
        )
    else:
        from aws_finops_dashboard.legacy_pdf_renderer import (
            render_dashboard_report as render_dashboard_report_impl,
        )
    render_dashboard_report_impl(doc, report)


def render_audit_report_pdf(
    pdf_output: object,
    audit_data_list: Sequence[Dict[str, str]],
    pdf_style: PdfStyle = "legacy",
) -> None:
    """Render the audit report PDF using the requested style."""
    report = build_audit_report(audit_data_list)
    doc = _build_doc_template(pdf_output)
    if pdf_style == "executive":
        from aws_finops_dashboard.executive_pdf_renderer import (
            render_audit_report as render_audit_report_impl,
        )
    else:
        from aws_finops_dashboard.legacy_pdf_renderer import (
            render_audit_report as render_audit_report_impl,
        )
    render_audit_report_impl(doc, report)


def _build_doc_template(pdf_output: object) -> SimpleDocTemplate:
    return SimpleDocTemplate(
        pdf_output,
        pagesize=(612.0, 792.0),
        leftMargin=36.0,
        rightMargin=36.0,
        topMargin=36.0,
        bottomMargin=36.0,
        allowSplitting=True,
    )
