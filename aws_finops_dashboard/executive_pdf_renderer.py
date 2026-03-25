"""Executive PDF renderer with cover page, sections, and polished templates."""

import os
from typing import List, Optional

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_LEFT
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.platypus import (
    BaseDocTemplate,
    Flowable,
    Frame,
    Image,
    NextPageTemplate,
    PageBreak,
    PageTemplate,
    Paragraph,
    Spacer,
    Table,
    TableStyle,
)

from aws_finops_dashboard.pdf_utils import bulletList, paragraphStyling
from aws_finops_dashboard.report_models import (
    AuditReport,
    ChartAsset,
    CostBreakdownItem,
    DashboardProfileReport,
    DashboardReport,
    KubernetesCostData,
)

styles = getSampleStyleSheet()

palette = {
    "ink": colors.HexColor("#163247"),
    "accent": colors.HexColor("#2E6E8E"),
    "accent_soft": colors.HexColor("#E7F0F5"),
    "line": colors.HexColor("#C9D8E3"),
    "muted": colors.HexColor("#5F7382"),
    "paper": colors.HexColor("#F8FBFD"),
    "warning": colors.HexColor("#6B4F2A"),
}

cover_kicker_style = ParagraphStyle(
    "ExecutiveCoverKicker",
    parent=styles["BodyText"],
    fontName="Helvetica-Bold",
    fontSize=10,
    leading=12,
    textColor=palette["accent"],
    alignment=TA_LEFT,
    spaceAfter=10,
)
cover_title_style = ParagraphStyle(
    "ExecutiveCoverTitle",
    parent=styles["Title"],
    fontName="Helvetica-Bold",
    fontSize=26,
    leading=30,
    textColor=palette["ink"],
    alignment=TA_LEFT,
    spaceAfter=10,
)
cover_meta_style = ParagraphStyle(
    "ExecutiveCoverMeta",
    parent=styles["BodyText"],
    fontSize=11,
    leading=15,
    textColor=palette["muted"],
    alignment=TA_LEFT,
)
section_title_style = ParagraphStyle(
    "ExecutiveSectionTitle",
    parent=styles["Heading1"],
    fontName="Helvetica-Bold",
    fontSize=18,
    leading=22,
    textColor=palette["ink"],
    alignment=TA_LEFT,
    spaceAfter=8,
)
section_intro_style = ParagraphStyle(
    "ExecutiveSectionIntro",
    parent=styles["BodyText"],
    fontSize=10,
    leading=14,
    textColor=palette["muted"],
    alignment=TA_LEFT,
    spaceAfter=12,
)
body_style = ParagraphStyle(
    "ExecutiveBody",
    parent=styles["BodyText"],
    fontSize=9,
    leading=12,
    textColor=palette["ink"],
)
small_caps_style = ParagraphStyle(
    "ExecutiveSmallCaps",
    parent=styles["BodyText"],
    fontName="Helvetica-Bold",
    fontSize=8,
    leading=10,
    textColor=palette["accent"],
    alignment=TA_CENTER,
)
metric_value_style = ParagraphStyle(
    "ExecutiveMetricValue",
    parent=styles["BodyText"],
    fontName="Helvetica-Bold",
    fontSize=16,
    leading=18,
    textColor=palette["ink"],
    alignment=TA_CENTER,
)
metric_label_style = ParagraphStyle(
    "ExecutiveMetricLabel",
    parent=styles["BodyText"],
    fontSize=8,
    leading=10,
    textColor=palette["muted"],
    alignment=TA_CENTER,
)
caption_style = ParagraphStyle(
    "ExecutiveCaption",
    parent=styles["BodyText"],
    fontSize=8,
    leading=10,
    textColor=palette["muted"],
    alignment=TA_LEFT,
)
footer_style = ParagraphStyle(
    "ExecutiveFooter",
    parent=styles["BodyText"],
    fontSize=8,
    leading=10,
    textColor=palette["muted"],
    alignment=TA_CENTER,
)


class ExecutiveDocTemplate(BaseDocTemplate):
    """Document template with separate cover and content page treatments."""

    def __init__(self, filename: object):
        super().__init__(
            filename,
            pagesize=letter,
            leftMargin=0.65 * inch,
            rightMargin=0.65 * inch,
            topMargin=0.75 * inch,
            bottomMargin=0.75 * inch,
        )
        content_frame = Frame(
            self.leftMargin,
            self.bottomMargin,
            self.width,
            self.height,
            id="content",
        )
        self.addPageTemplates(
            [
                PageTemplate(
                    id="Cover",
                    frames=[content_frame],
                    onPage=self._draw_cover_page,
                ),
                PageTemplate(
                    id="Content",
                    frames=[content_frame],
                    onPage=self._draw_content_page,
                ),
            ]
        )

    def _draw_cover_page(self, canvas, doc) -> None:
        canvas.saveState()
        width, height = letter
        canvas.setFillColor(palette["paper"])
        canvas.rect(0, 0, width, height, stroke=0, fill=1)
        canvas.setFillColor(palette["accent_soft"])
        canvas.rect(0, height - 1.15 * inch, width, 1.15 * inch, stroke=0, fill=1)
        canvas.restoreState()

    def _draw_content_page(self, canvas, doc) -> None:
        canvas.saveState()
        width, height = letter
        canvas.setStrokeColor(palette["line"])
        canvas.setLineWidth(0.6)
        canvas.line(
            self.leftMargin,
            height - 0.48 * inch,
            width - self.rightMargin,
            height - 0.48 * inch,
        )
        canvas.setFont("Helvetica-Bold", 9)
        canvas.setFillColor(palette["accent"])
        canvas.drawString(self.leftMargin, height - 0.38 * inch, "AWS FinOps Dashboard")
        canvas.setFont("Helvetica", 8)
        canvas.setFillColor(palette["muted"])
        canvas.drawRightString(
            width - self.rightMargin,
            0.42 * inch,
            f"Page {doc.page}",
        )
        canvas.restoreState()


def render_dashboard_report(doc: object, report: DashboardReport) -> None:
    """Render the cost dashboard in the executive style."""
    executive_doc = ExecutiveDocTemplate(doc.filename) if hasattr(doc, "filename") else ExecutiveDocTemplate(doc)
    story: List[Flowable] = []

    story.extend(_build_cover_story(report))
    story.append(NextPageTemplate("Content"))
    story.append(PageBreak())

    story.extend(
        _build_section_header(
            "Executive Summary",
            "A concise view of cost movement, top drivers, and immediate FinOps signals.",
        )
    )
    story.extend(_build_summary_story(report))
    story.append(PageBreak())

    story.extend(
        _build_section_header(
            "Cost Trend Charts",
            "Pre-generated trend visuals embedded for executive review.",
        )
    )
    story.extend(_build_chart_story(report.options.chart_assets))
    story.append(PageBreak())

    story.extend(
        _build_section_header(
            "Cost Breakdown by Account/Project",
            "Current and prior-period spend by included AWS profile or combined account view.",
        )
    )
    story.extend(_build_account_breakdown_story(report))
    story.append(PageBreak())

    story.extend(
        _build_section_header(
            "Top Services by Cost",
            "Highest-spend AWS services aggregated across the report scope.",
        )
    )
    story.extend(_build_top_services_story(report))
    story.append(PageBreak())

    if report.kubernetes_costs is not None:
        story.extend(
            _build_section_header(
                "Kubernetes Costs",
                "Optional Kubernetes cost allocation data sourced from OpenCost.",
            )
        )
        story.extend(_build_kubernetes_story(report.kubernetes_costs))
        story.append(PageBreak())

    story.extend(
        _build_section_header(
            "FinOps Observations",
            "Suggested talking points drawn from the current report payload.",
        )
    )
    story.extend(_build_observations_story(report))
    story.append(PageBreak())

    story.extend(
        _build_section_header(
            "Appendix",
            "Detailed profile-level metrics, service mix, budgets, and inventory notes.",
        )
    )
    story.extend(_build_appendix_story(report))

    executive_doc.build(story)


def render_audit_report(doc: object, report: AuditReport) -> None:
    """Render the audit report in the executive style."""
    executive_doc = ExecutiveDocTemplate(doc.filename) if hasattr(doc, "filename") else ExecutiveDocTemplate(doc)
    story: List[Flowable] = []
    story.extend(_build_audit_cover_story(report))
    story.append(NextPageTemplate("Content"))
    story.append(PageBreak())
    story.extend(
        _build_section_header(
            "Audit Findings",
            "Operational waste, hygiene issues, and budget alert context by profile.",
        )
    )
    for index, profile in enumerate(report.profiles):
        story.append(_profile_banner(profile.profile, profile.account_id))
        story.append(Spacer(1, 0.15 * inch))
        for section in profile.sections:
            story.append(Paragraph(section.title, body_style))
            story.append(bulletList(section.items))
            story.append(Spacer(1, 0.12 * inch))
        if index < len(report.profiles) - 1:
            story.append(Spacer(1, 0.2 * inch))
    if report.footer_note:
        story.append(Spacer(1, 0.12 * inch))
        story.append(Paragraph(report.footer_note, caption_style))
    executive_doc.build(story)


def _build_cover_story(report: DashboardReport) -> List[Flowable]:
    confidentiality = (
        report.options.confidentiality_notice or "Confidential - Internal Use Only"
    )
    story: List[Flowable] = [Spacer(1, 0.5 * inch)]
    logo = _build_logo(report.options.logo_path, width=1.6 * inch, height=0.8 * inch)
    if logo is not None:
        story.extend([logo, Spacer(1, 0.35 * inch)])
    story.append(Paragraph("EXECUTIVE COST REPORT", cover_kicker_style))
    story.append(Paragraph(report.title, cover_title_style))
    story.append(
        Paragraph(
            (
                f"<b>Reporting period</b><br/>{report.previous_period.date_range}"
                f"<br/>{report.current_period.date_range}<br/><br/>"
                f"<b>Generated</b><br/>{report.generated_at:%Y-%m-%d %H:%M:%S}"
            ),
            cover_meta_style,
        )
    )
    story.append(Spacer(1, 2.7 * inch))
    story.append(_confidentiality_block(confidentiality))
    return story


def _build_audit_cover_story(report: AuditReport) -> List[Flowable]:
    story: List[Flowable] = [Spacer(1, 1.0 * inch)]
    story.append(Paragraph("EXECUTIVE AUDIT REPORT", cover_kicker_style))
    story.append(Paragraph(report.title, cover_title_style))
    story.append(
        Paragraph(
            f"<b>Generated</b><br/>{report.generated_at:%Y-%m-%d %H:%M:%S}",
            cover_meta_style,
        )
    )
    story.append(Spacer(1, 3.6 * inch))
    story.append(_confidentiality_block("Confidential - Internal Use Only"))
    return story


def _build_section_header(title: str, intro: str) -> List[Flowable]:
    return [
        Paragraph(title, section_title_style),
        Paragraph(intro, section_intro_style),
    ]


def _build_summary_story(report: DashboardReport) -> List[Flowable]:
    summary = report.executive_summary
    metrics = Table(
        [
            [
                _metric_card(f"${summary.total_cost:,.2f}", "Total Cost"),
                _metric_card(
                    f"${summary.previous_period_cost:,.2f}",
                    "Previous Period Cost",
                ),
            ],
            [
                _metric_card(f"${summary.delta_amount:,.2f}", "Delta Amount"),
                _metric_card(
                    _format_delta(summary.delta_percentage),
                    "Delta Percentage",
                ),
            ],
        ],
        colWidths=[3.1 * inch, 3.1 * inch],
        rowHeights=[1.05 * inch, 1.05 * inch],
        hAlign="LEFT",
    )
    metrics.setStyle(TableStyle([("VALIGN", (0, 0), (-1, -1), "TOP")]))
    top_driver = _text_panel(
        "Top Cost Driver",
        [summary.top_cost_driver],
        width=6.2 * inch,
    )
    return [
        metrics,
        Spacer(1, 0.2 * inch),
        top_driver,
    ]


def _build_chart_story(chart_assets: Optional[List[ChartAsset]]) -> List[Flowable]:
    if not chart_assets:
        return [
            Paragraph(
                "No chart assets were available for this report, so the chart section was skipped.",
                body_style,
            )
        ]

    story: List[Flowable] = []
    valid_chart_found = False
    for asset in chart_assets:
        if asset.note:
            story.append(Paragraph(asset.title, body_style))
            story.append(Spacer(1, 0.05 * inch))
            story.append(Paragraph(asset.note, caption_style))
            story.append(Spacer(1, 0.18 * inch))
            continue
        if not asset.path or not os.path.exists(asset.path):
            story.append(
                Paragraph(
                    f"Chart asset not found: {asset.path or 'unknown path'}",
                    caption_style,
                )
            )
            story.append(Spacer(1, 0.12 * inch))
            continue
        valid_chart_found = True
        story.append(Paragraph(asset.title, body_style))
        story.append(Spacer(1, 0.08 * inch))
        chart = Image(asset.path)
        chart._restrictSize(6.2 * inch, 3.8 * inch)
        story.append(chart)
        story.append(Spacer(1, 0.08 * inch))
        story.append(Paragraph(asset.path, caption_style))
        story.append(Spacer(1, 0.22 * inch))
    if not valid_chart_found:
        story.append(
            Paragraph("No readable chart images were available to embed.", body_style)
        )
    return story


def _build_account_breakdown_story(report: DashboardReport) -> List[Flowable]:
    rows = [["Account / Project", "Previous", "Current", "Delta", "Delta %"]]
    for profile in report.profiles:
        delta_amount = profile.current_period_cost - profile.previous_period_cost
        rows.append(
            [
                f"{profile.profile}\n{profile.account_id}",
                f"${profile.previous_period_cost:,.2f}",
                f"${profile.current_period_cost:,.2f}",
                f"${delta_amount:,.2f}",
                _format_delta(profile.percent_change_in_total_cost),
            ]
        )
    return [_styled_data_table(rows, [2.35 * inch, 1.0 * inch, 1.0 * inch, 0.95 * inch, 0.9 * inch])]


def _build_top_services_story(report: DashboardReport) -> List[Flowable]:
    rows = [["Service", "Current Period Cost"]]
    for item in report.top_services:
        rows.append([item.name, f"${item.amount:,.2f}"])
    if len(rows) == 1:
        rows.append(["No services", "$0.00"])
    return [_styled_data_table(rows, [4.7 * inch, 1.5 * inch])]


def _build_observations_story(report: DashboardReport) -> List[Flowable]:
    content: List[Flowable] = [bulletList([insight.message for insight in report.insights])]
    content.append(Spacer(1, 0.2 * inch))
    k8s_context = (
        [f"Kubernetes source: {report.kubernetes_costs.source}"]
        if report.kubernetes_costs is not None
        else []
    )
    content.append(
        _text_panel(
            "Executive Context",
            [
                f"Included accounts/projects: {len(report.profiles)}",
                f"Tracked top services: {len(report.top_services)}",
                f"Generated timestamp: {report.generated_at:%Y-%m-%d %H:%M:%S}",
                *k8s_context,
            ],
            width=6.2 * inch,
        )
    )
    return content


def _build_appendix_story(report: DashboardReport) -> List[Flowable]:
    story: List[Flowable] = []
    for index, profile in enumerate(report.profiles):
        story.append(_profile_banner(profile.profile, profile.account_id))
        story.append(Spacer(1, 0.12 * inch))
        story.append(
            _text_panel(
                "Profile Metrics",
                [
                    f"Current period cost: ${profile.current_period_cost:,.2f}",
                    f"Previous period cost: ${profile.previous_period_cost:,.2f}",
                    f"Change: {_format_delta(profile.percent_change_in_total_cost)}",
                ],
                width=6.2 * inch,
            )
        )
        story.append(Spacer(1, 0.12 * inch))
        story.append(
            _two_column_panels(
                _text_panel(
                    "Current Services",
                    _format_cost_lines(profile.current_services) or ["No costs"],
                    width=3.0 * inch,
                ),
                _text_panel(
                    "Previous Services",
                    _format_cost_lines(profile.previous_services) or ["No costs"],
                    width=3.0 * inch,
                ),
            )
        )
        story.append(Spacer(1, 0.12 * inch))
        story.append(
            _two_column_panels(
                _text_panel(
                    "Budget Status",
                    profile.budget_items or ["No budgets"],
                    width=3.0 * inch,
                ),
                _text_panel(
                    "EC2 Inventory",
                    profile.inventory_items or ["No instances"],
                    width=3.0 * inch,
                ),
            )
        )
        if index < len(report.profiles) - 1:
            story.append(Spacer(1, 0.2 * inch))
    if report.kubernetes_costs is not None:
        story.append(Spacer(1, 0.16 * inch))
        story.append(
            _text_panel(
                "Kubernetes Appendix",
                _kubernetes_appendix_lines(report.kubernetes_costs),
                width=6.2 * inch,
            )
        )
    return story


def _build_kubernetes_story(kubernetes_costs: KubernetesCostData) -> List[Flowable]:
    summary_rows = [
        ["Metric", "Cost"],
        ["Total Cluster Cost", f"${kubernetes_costs.total_cost:,.2f}"],
        ["Idle Cost", _format_optional_cost(kubernetes_costs.idle_cost)],
        ["Shared Cost", _format_optional_cost(kubernetes_costs.shared_cost)],
        ["Unallocated Cost", _format_optional_cost(kubernetes_costs.unallocated_cost)],
    ]

    story: List[Flowable] = [
        _styled_data_table(summary_rows, [4.7 * inch, 1.5 * inch]),
        Spacer(1, 0.18 * inch),
    ]
    if kubernetes_costs.warnings:
        story.append(_text_panel("Kubernetes Notes", kubernetes_costs.warnings, 6.2 * inch))
        story.append(Spacer(1, 0.18 * inch))

    story.append(
        _two_column_panels(
            _text_panel(
                "Top Namespaces",
                _format_kubernetes_items(kubernetes_costs.namespace_costs)
                or ["No namespace costs available"],
                width=3.0 * inch,
            ),
            _text_panel(
                "Top Workloads",
                _format_kubernetes_items(kubernetes_costs.workload_costs)
                or ["No workload costs available"],
                width=3.0 * inch,
            ),
        )
    )
    return story


def _metric_card(value: str, label: str) -> Table:
    card = Table(
        [
            [Paragraph(label.upper(), small_caps_style)],
            [Paragraph(value, metric_value_style)],
        ],
        colWidths=[2.85 * inch],
    )
    card.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, -1), colors.white),
                ("BOX", (0, 0), (-1, -1), 0.8, palette["line"]),
                ("LEFTPADDING", (0, 0), (-1, -1), 12),
                ("RIGHTPADDING", (0, 0), (-1, -1), 12),
                ("TOPPADDING", (0, 0), (-1, -1), 12),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 12),
            ]
        )
    )
    return card


def _text_panel(title: str, lines: List[str], width: float) -> Table:
    body = "<br/>".join(_escape_lines(lines))
    panel = Table(
        [[paragraphStyling(f"<b>{title}</b><br/><br/>{body}", font_size=9, leading=12)]],
        colWidths=[width - 0.3 * inch],
    )
    panel.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, -1), colors.white),
                ("BOX", (0, 0), (-1, -1), 0.8, palette["line"]),
                ("LEFTPADDING", (0, 0), (-1, -1), 12),
                ("RIGHTPADDING", (0, 0), (-1, -1), 12),
                ("TOPPADDING", (0, 0), (-1, -1), 10),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 10),
            ]
        )
    )
    return panel


def _two_column_panels(left: Table, right: Table) -> Table:
    table = Table([[left, right]], colWidths=[3.1 * inch, 3.1 * inch], hAlign="LEFT")
    table.setStyle(TableStyle([("VALIGN", (0, 0), (-1, -1), "TOP")]))
    return table


def _styled_data_table(rows: List[List[str]], col_widths: List[float]) -> Table:
    table = Table(rows, colWidths=col_widths, hAlign="LEFT", repeatRows=1)
    table.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, 0), palette["accent_soft"]),
                ("TEXTCOLOR", (0, 0), (-1, 0), palette["ink"]),
                ("FONTNAME", (0, 0), (-1, 0), "Helvetica-Bold"),
                ("FONTNAME", (0, 1), (-1, -1), "Helvetica"),
                ("FONTSIZE", (0, 0), (-1, -1), 9),
                ("LEADING", (0, 0), (-1, -1), 11),
                ("GRID", (0, 0), (-1, -1), 0.5, palette["line"]),
                ("VALIGN", (0, 0), (-1, -1), "TOP"),
                ("LEFTPADDING", (0, 0), (-1, -1), 8),
                ("RIGHTPADDING", (0, 0), (-1, -1), 8),
                ("TOPPADDING", (0, 0), (-1, -1), 7),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 7),
                ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, palette["paper"]]),
            ]
        )
    )
    return table


def _profile_banner(profile: str, account_id: str) -> Table:
    banner = Table(
        [
            [
                Paragraph(f"<b>Profile</b><br/>{profile}", body_style),
                Paragraph(f"<b>Account</b><br/>{account_id}", body_style),
            ]
        ],
        colWidths=[3.05 * inch, 3.05 * inch],
    )
    banner.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, -1), palette["accent_soft"]),
                ("BOX", (0, 0), (-1, -1), 0.8, palette["line"]),
                ("LEFTPADDING", (0, 0), (-1, -1), 10),
                ("RIGHTPADDING", (0, 0), (-1, -1), 10),
                ("TOPPADDING", (0, 0), (-1, -1), 8),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 8),
            ]
        )
    )
    return banner


def _confidentiality_block(text: str) -> Table:
    block = Table(
        [[Paragraph(f"<b>Confidentiality</b><br/><br/>{_escape_lines([text])[0]}", body_style)]],
        colWidths=[6.0 * inch],
    )
    block.setStyle(
        TableStyle(
            [
                ("BACKGROUND", (0, 0), (-1, -1), colors.white),
                ("BOX", (0, 0), (-1, -1), 0.9, palette["warning"]),
                ("LEFTPADDING", (0, 0), (-1, -1), 14),
                ("RIGHTPADDING", (0, 0), (-1, -1), 14),
                ("TOPPADDING", (0, 0), (-1, -1), 12),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 12),
            ]
        )
    )
    return block


def _build_logo(
    path: Optional[str],
    width: float,
    height: float,
) -> Optional[Image]:
    if not path or not os.path.exists(path):
        return None
    image = Image(path)
    image._restrictSize(width, height)
    return image


def _format_cost_lines(items: List[CostBreakdownItem]) -> List[str]:
    return [f"{item.name}: ${item.amount:,.2f}" for item in items]


def _format_kubernetes_items(items) -> List[str]:
    return [f"{item.name}: ${item.cost:,.2f}" for item in items]


def _format_optional_cost(value: Optional[float]) -> str:
    if value is None:
        return "N/A"
    return f"${value:,.2f}"


def _kubernetes_appendix_lines(kubernetes_costs: KubernetesCostData) -> List[str]:
    lines = [
        f"Source: {kubernetes_costs.source}",
        f"Window: {kubernetes_costs.window}",
        f"Total cluster cost: ${kubernetes_costs.total_cost:,.2f}",
    ]
    lines.extend(
        _format_kubernetes_items(kubernetes_costs.namespace_costs[:5])
        or ["No namespace costs available"]
    )
    return lines


def _format_delta(value: Optional[float]) -> str:
    if value is None:
        return "N/A"
    if value > 0:
        return f"+{value:.2f}%"
    return f"{value:.2f}%"


def _escape_lines(lines: List[str]) -> List[str]:
    return [
        line.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
        for line in lines
    ]
