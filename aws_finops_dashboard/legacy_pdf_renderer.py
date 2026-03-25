"""Backward-compatible PDF renderer preserving the existing report layout."""

from typing import List

from reportlab.lib import colors
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.platypus import (
    Flowable,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)

from aws_finops_dashboard.pdf_utils import (
    bulletList,
    keyValueTable,
    miniHeader,
    paragraphStyling,
    split_to_items,
)
from aws_finops_dashboard.report_models import AuditReport, DashboardReport

styles = getSampleStyleSheet()
pdf_footer_style = ParagraphStyle(
    name="PDF_Footer",
    parent=styles["Normal"],
    fontSize=8,
    textColor=colors.grey,
    alignment=1,
    leading=10,
)


def render_dashboard_report(doc: SimpleDocTemplate, report: DashboardReport) -> None:
    """Render the cost dashboard using the original PDF layout."""
    elements: List[Flowable] = []
    elements.append(Paragraph(report.title, styles["Title"]))
    elements.append(Spacer(1, 10))
    elements.append(
        paragraphStyling(
            f"<b>{report.previous_period.label}:</b> {report.previous_period.date_range}<br/>"
            f"<b>{report.current_period.label}:</b> {report.current_period.date_range}"
        )
    )
    elements.append(Spacer(1, 6))

    for idx, row in enumerate(report.profiles):
        elements.extend(_profile_header(doc, row.profile, row.account_id))
        elements.append(
            keyValueTable(
                [
                    (
                        report.previous_period.label,
                        f"<b>${row.previous_period_cost:.2f}</b>",
                    ),
                    (
                        report.current_period.label,
                        f"<b>${row.current_period_cost:.2f}</b>",
                    ),
                ]
            )
        )
        elements.append(Spacer(1, 6))

        elements.append(miniHeader("Previous Period Cost By Service"))
        elements.append(
            bulletList(
                [f"{item.name}: ${item.amount:,.2f}" for item in row.previous_services]
                or ["No costs"]
            )
        )
        elements.append(Spacer(1, 6))

        elements.append(miniHeader("Current Period Cost By Service"))
        elements.append(
            bulletList(
                [f"{item.name}: ${item.amount:,.2f}" for item in row.current_services]
                or ["No costs"]
            )
        )
        elements.append(Spacer(1, 6))

        elements.append(miniHeader("Budget Status"))
        elements.append(bulletList(row.budget_items or ["No budgets"]))
        elements.append(Spacer(1, 6))

        elements.append(miniHeader("EC2 Instances"))
        elements.append(bulletList(row.inventory_items))

        if idx < len(report.profiles) - 1:
            elements.append(Spacer(1, 14))

    elements.append(Spacer(1, 8))
    footer_text = (
        "This report is generated using AWS FinOps Dashboard (CLI) "
        f"\u00a9 2025 on {report.generated_at:%Y-%m-%d %H:%M:%S}"
    )
    elements.append(Paragraph(footer_text, pdf_footer_style))
    doc.build(elements)


def render_audit_report(doc: SimpleDocTemplate, report: AuditReport) -> None:
    """Render the audit report using the original PDF layout."""
    elements: List[Flowable] = []
    elements.append(Paragraph(report.title, styles["Title"]))
    elements.append(Spacer(1, 8))

    for idx, profile in enumerate(report.profiles):
        elements.extend(_profile_header(doc, profile.profile, profile.account_id))
        for section in profile.sections:
            elements.append(miniHeader(section.title))
            elements.append(bulletList(split_to_items("\n".join(section.items))))
            elements.append(Spacer(1, 6))
        if idx < len(report.profiles) - 1:
            elements.append(Spacer(1, 10))

    elements.append(Spacer(1, 8))
    if report.footer_note:
        elements.append(Paragraph(report.footer_note, pdf_footer_style))
    footer_text = (
        "This audit report is generated using AWS FinOps Dashboard (CLI) "
        f"\u00a9 2025 on {report.generated_at:%Y-%m-%d %H:%M:%S}"
    )
    elements.append(Paragraph(footer_text, pdf_footer_style))
    doc.build(elements)


def _profile_header(
    doc: SimpleDocTemplate, profile: str, account_id: str
) -> List[Flowable]:
    header_tbl = Table(
        [
            [
                paragraphStyling(
                    f"<b>Profile:</b> {profile}  &nbsp;&nbsp;&nbsp; "
                    f"<b>Account:</b> {account_id}"
                )
            ]
        ],
        colWidths=[doc.width],
        hAlign="LEFT",
    )
    header_tbl.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, -1), colors.whitesmoke),
                ("BOX", (0, 0), (-1, -1), 0.25, colors.grey),
                ("LEFTPADDING", (0, 0), (-1, -1), 6),
                ("RIGHTPADDING", (0, 0), (-1, -1), 6),
                ("TOPPADDING", (0, 0), (-1, -1), 4),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
            ]
        )
    )
    return [header_tbl, Spacer(1, 6)]
