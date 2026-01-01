from reportlab.platypus import (
    Paragraph, 
    Table, 
    TableStyle,
    ListFlowable, 
    ListItem, 
)
from reportlab.lib.units import inch
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from typing import List

styles = getSampleStyleSheet()


def paragraphStyling(text: str, style_name="BodyText", font_size=9, leading=11):
    base = styles[style_name]
    st = ParagraphStyle(
        f"{style_name}_cell",
        parent=base,
        fontSize=font_size,
        leading=leading,
    )
    return Paragraph(text, st)


def miniHeader(text: str):
    return paragraphStyling(f"<b>{text}</b>", style_name="BodyText", font_size=9, leading=11)


def keyValueTable(rows, colWidths=None):
    # rows = List[Tuple[label, value]]
    data = [[paragraphStyling(f"<b>{k}</b>"), paragraphStyling(v)] for k, v in rows]
    t = Table(data, colWidths=colWidths or [1.6*inch, 5.8*inch], hAlign="LEFT")
    t.setStyle(TableStyle([
        ("FONTNAME", (0, 0), (-1, -1), "Helvetica"),
        ("FONTSIZE", (0, 0), (-1, -1), 9),
        ("LEADING", (0, 0), (-1, -1), 11),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 3),
        ("TOPPADDING", (0, 0), (-1, -1), 3),
    ]))
    return t


def bulletList(items):
    # items: List[str]
    # Use smaller font and allow splitting across pages
    styled = [ListItem(paragraphStyling(i, font_size=9, leading=11), leftIndent=6) for i in items]
    return ListFlowable(styled, bulletType="bullet", start="•", leftIndent=12)


def formatServicesForList(services):
    # services: List[Tuple[str, float]]
    if not services:
        return ["No costs"]
    return [f"{svc}: ${cost:,.2f}" for svc, cost in services]


def split_to_items(value: str) -> List[str]:
    """Turn a possibly multiline string into bullet items (safe for Paragraph)."""
    if not value:
        return ["None"]
    items = [line.strip() for line in value.splitlines() if line.strip()]
    return items or ["None"]