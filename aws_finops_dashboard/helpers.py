import csv
import json
import os
import re
import sys
from datetime import datetime
from typing import Any, Dict, List, Optional
from io import BytesIO, StringIO
from boto3.session import Session
from botocore.exceptions import ClientError

if sys.version_info >= (3, 11):
    import tomllib
else:
    try:
        import tomli as tomllib
    except ImportError:
        tomllib = None

import yaml
from reportlab.lib import colors
from reportlab.lib.pagesizes import portrait, letter
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.platypus import (
    Flowable,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)
from reportlab.lib.units import inch
from rich.console import Console

from aws_finops_dashboard.types import ProfileData
from aws_finops_dashboard.pdf_utils import (
    paragraphStyling,
    miniHeader,
    keyValueTable,
    bulletList,
    formatServicesForList,
    split_to_items,
)

console = Console()


styles = getSampleStyleSheet()

# Custom style for the footer
pdf_footer_style = ParagraphStyle(
    name="PDF_Footer",
    parent=styles["Normal"],
    fontSize=8,
    textColor=colors.grey,
    alignment=1,
    leading=10,
)

def upload_to_s3(
    content: bytes,
    bucket: str,
    key: str,
    session: Session,
    content_type: Optional[str] = None,
) -> Optional[str]:
    try:
        s3_client = session.client("s3")

        if not content_type:
            if key.endswith(".pdf"):
                content_type = "application/pdf"
            elif key.endswith(".csv"):
                content_type = "text/csv"
            elif key.endswith(".json"):
                content_type = "application/json"
            
        s3_client.put_object(
            Bucket=bucket,
            Key=key,
            Body=content,
            ContentType=content_type,
        )

        s3_path = f"s3://{bucket}/{key}"
        return s3_path
    
    except ClientError as e:
        console.print(f"[bold red]Error uploading to S3: {str(e)}[/]")
        return None
    except Exception as e:
        console.print(f"[bold red]Error uploading to S3: {str(e)}[/]")
        return None

def export_audit_report_to_pdf(
    audit_data_list: List[Dict[str, str]],
    file_name: str = "audit_report",
    path: Optional[str] = None,
    export_handler=None,
) -> Optional[str]:
    """
    Text-mode audit report: one section per profile with small flowables (lists/paras),
    so content wraps and paginates cleanly.
    """
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{file_name}_{timestamp}.pdf"

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=path)

        # Get output destination (BytesIO for S3, file path for local)
        pdf_output = export_handler.get_pdf_output(base_filename)

        doc = SimpleDocTemplate(
            pdf_output,
            pagesize=portrait(letter),
            leftMargin=0.5*inch,
            rightMargin=0.5*inch,
            topMargin=0.5*inch,
            bottomMargin=0.5*inch,
            allowSplitting=True,
        )

        elements: List[Flowable] = []
        elements.append(Paragraph("AWS FinOps Dashboard (Audit Report)", styles["Title"]))
        elements.append(Spacer(1, 8))

        for idx, row in enumerate(audit_data_list):
            # Header card per profile
            header_tbl = Table(
                [[paragraphStyling(f"<b>Profile:</b> {row['profile']}  &nbsp;&nbsp;&nbsp; "
                     f"<b>Account:</b> {row['account_id']}")]],
                colWidths=[doc.width],
                hAlign="LEFT",
            )
            header_tbl.setStyle(TableStyle([
                ("BACKGROUND", (0, 0), (-1, -1), colors.whitesmoke),
                ("BOX", (0, 0), (-1, -1), 0.25, colors.grey),
                ("LEFTPADDING", (0, 0), (-1, -1), 6),
                ("RIGHTPADDING", (0, 0), (-1, -1), 6),
                ("TOPPADDING", (0, 0), (-1, -1), 4),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
            ]))
            elements.append(header_tbl)
            elements.append(Spacer(1, 6))

            # Sections (each as a bulleted list)
            sections = [
                ("Untagged Resources", split_to_items(row.get("untagged_resources", ""))),
                ("Stopped EC2 Instances", split_to_items(row.get("stopped_instances", ""))),
                ("Unused Volumes", split_to_items(row.get("unused_volumes", ""))),
                ("Unused EIPs", split_to_items(row.get("unused_eips", ""))),
                ("Budget Alerts", split_to_items(row.get("budget_alerts", ""))),
            ]

            for title, items in sections:
                elements.append(miniHeader(title))
                elements.append(bulletList(items))
                elements.append(Spacer(1, 6))

            if idx < len(audit_data_list) - 1:
                elements.append(Spacer(1, 10))

        elements.append(Spacer(1, 8))
        footer_note = "Note: This report lists untagged EC2, RDS, Lambda, ELBv2 only."
        elements.append(Paragraph(footer_note, pdf_footer_style))
        footer_text = f"This audit report is generated using AWS FinOps Dashboard (CLI) \u00a9 2025 on {datetime.now():%Y-%m-%d %H:%M:%S}"
        elements.append(Paragraph(footer_text, pdf_footer_style))

        doc.build(elements)

        # Finalize PDF export
        return export_handler.finalize_pdf(pdf_output, base_filename)

    except Exception as e:
        console.print(f"[bold red]Error exporting audit report to PDF: {str(e)}[/]")
        return None


def clean_rich_tags(text: str) -> str:
    """
    Clean the rich text before writing the data to a pdf.

    :param text: The rich text to clean.
    :return: Cleaned text.
    """
    return re.sub(r"\[/?[a-zA-Z0-9#_]*\]", "", text)


def export_audit_report_to_csv(
    audit_data_list: List[Dict[str, str]],
    file_name: str = "audit_report",
    path: Optional[str] = None,
    export_handler=None,
) -> Optional[str]:
    """Export the audit report to a CSV file or S3."""
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{file_name}_{timestamp}.csv"

        csv_buffer = StringIO()

        headers = [
            "Profile",
            "Account ID",
            "Untagged Resources",
            "Stopped EC2 Instances",
            "Unused Volumes",
            "Unused EIPs",
            "Budget Alerts",
        ]
        data_keys = [
            "profile",
            "account_id",
            "untagged_resources",
            "stopped_instances",
            "unused_volumes",
            "unused_eips",
            "budget_alerts",
        ]

        writer = csv.writer(csv_buffer)
        writer.writerow(headers)
        for item in audit_data_list:
            writer.writerow([item.get(key, "") for key in data_keys])

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=path)

        csv_content = csv_buffer.getvalue().encode("utf-8")
        saved_path = export_handler.save(csv_content, base_filename, "text/csv")

        return saved_path
    except Exception as e:
        console.print(f"[bold red]Error exporting audit report to CSV: {str(e)}[/]")
        return None

def export_audit_report_to_json(
    raw_audit_data: List[Dict[str, Any]],
    file_name: str = "audit_report",
    path: Optional[str] = None,
    export_handler=None,
) -> Optional[str]:
    """Export the audit report to a JSON file or S3."""
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{file_name}_{timestamp}.json"

        json_content = json.dumps(raw_audit_data, indent=4).encode("utf-8")

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=path)

        saved_path = export_handler.save(json_content, base_filename, "application/json")

        return saved_path
    except Exception as e:
        console.print(f"[bold red]Error exporting audit report to JSON: {str(e)}[/]")
        return None
    
def export_trend_data_to_json(
    trend_data: List[Dict[str, Any]],
    file_name: str = "trend_data",
    path: Optional[str] = None,
    export_handler=None,
) -> Optional[str]:
    """Export trend data to a JSON file or S3."""
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{file_name}_{timestamp}.json"

        json_content = json.dumps(trend_data, indent=4).encode("utf-8")

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=path)

        saved_path = export_handler.save(json_content, base_filename, "application/json")

        return saved_path
    except Exception as e:
        console.print(f"[bold red]Error exporting trend data to JSON: {str(e)}[/]")
        return None
    
def export_cost_dashboard_to_pdf(
    data: List[ProfileData],
    filename: str,
    output_dir: Optional[str] = None,
    previous_period_dates: str = "N/A",
    current_period_dates: str = "N/A",
    export_handler=None,
) -> Optional[str]:
    from aws_finops_dashboard.export_handler import ExportHandler

    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.pdf"

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=output_dir)

        # Get output destination (BytesIO for S3, file path for local)
        pdf_output = export_handler.get_pdf_output(base_filename)

        doc = SimpleDocTemplate(
            pdf_output,
            pagesize=portrait(letter),
            leftMargin=0.5*inch,
            rightMargin=0.5*inch,
            topMargin=0.5*inch,
            bottomMargin=0.5*inch,
            allowSplitting=True,
        )

        elements: List[Flowable] = []
        elements.append(Paragraph("AWS FinOps Dashboard (Cost Report)", styles["Title"]))
        elements.append(Spacer(1, 10))

        elements.append(paragraphStyling(f"<b>Previous Period:</b> {previous_period_dates}<br/><b>Current Period:</b> {current_period_dates}"))
        elements.append(Spacer(1, 6))

        for idx, row in enumerate(data):
            header_tbl = Table(
                [[paragraphStyling(f"<b>Profile:</b> {row['profile']}  &nbsp;&nbsp;&nbsp; "
                     f"<b>Account:</b> {row['account_id']}")]],
                colWidths=[doc.width],
                hAlign="LEFT",
            )
            header_tbl.setStyle(TableStyle([
                ("BACKGROUND", (0, 0), (-1, -1), colors.whitesmoke),
                ("BOX", (0, 0), (-1, -1), 0.25, colors.grey),
                ("LEFTPADDING", (0, 0), (-1, -1), 6),
                ("RIGHTPADDING", (0, 0), (-1, -1), 6),
                ("TOPPADDING", (0, 0), (-1, -1), 4),
                ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
            ]))
            elements.append(header_tbl)
            elements.append(Spacer(1, 6))

            kv_rows = [
                ("Previous Period Cost", f"<b>${row['last_month']:.2f}</b>"),
                ("Current Period Cost", f"<b>${row['current_month']:.2f}</b>"),
            ]
            elements.append(keyValueTable(kv_rows))
            elements.append(Spacer(1, 6))

            elements.append(miniHeader("Previous Period Cost By Service"))
            prev_svc_items = formatServicesForList(row.get("previous_service_costs", []))
            elements.append(bulletList(prev_svc_items))
            elements.append(Spacer(1, 6))

            elements.append(miniHeader("Current Period Cost By Service"))
            svc_items = formatServicesForList(row["service_costs"])
            elements.append(bulletList(svc_items))
            elements.append(Spacer(1, 6))

            elements.append(miniHeader("Budget Status"))
            budgets = row["budget_info"] if row["budget_info"] else ["No budgets"]
            elements.append(bulletList(budgets))
            elements.append(Spacer(1, 6))

            elements.append(miniHeader("EC2 Instances"))
            ec2_items = [f"{state}: {count}" for state, count in row["ec2_summary"].items() if count > 0] or ["No instances"]
            elements.append(bulletList(ec2_items))

            if idx < len(data) - 1:
                elements.append(Spacer(1, 14))

        elements.append(Spacer(1, 8))
        footer_text = f"This report is generated using AWS FinOps Dashboard (CLI) \u00a9 2025 on {datetime.now():%Y-%m-%d %H:%M:%S}"
        elements.append(Paragraph(footer_text, pdf_footer_style))

        doc.build(elements)

        # Finalize PDF export
        return export_handler.finalize_pdf(pdf_output, base_filename)
    except Exception as e:
        console.print(f"[bold red]Error exporting to PDF: {str(e)}[/]")
        return None


def load_config_file(file_path: str) -> Optional[Dict[str, Any]]:
    """Load configuration from TOML, YAML, or JSON file."""
    _, file_extension = os.path.splitext(file_path)
    file_extension = file_extension.lower()

    try:
        with open(file_path, "rb" if file_extension == ".toml" else "r") as f:
            if file_extension == ".toml":
                if tomllib is None:
                    console.print(
                        f"[bold red]Error: TOML library (tomli) not installed for Python < 3.11. Please install it.[/]"
                    )
                    return None
                loaded_data = tomllib.load(f)
                if isinstance(loaded_data, dict):
                    return loaded_data
                console.print(
                    f"[bold red]Error: TOML file {file_path} did not load as a dictionary.[/]"
                )
                return None
            elif file_extension in [".yaml", ".yml"]:
                loaded_data = yaml.safe_load(f)
                if isinstance(loaded_data, dict):
                    return loaded_data
                console.print(
                    f"[bold red]Error: YAML file {file_path} did not load as a dictionary.[/]"
                )
                return None
            elif file_extension == ".json":
                loaded_data = json.load(f)
                if isinstance(loaded_data, dict):
                    return loaded_data
                console.print(
                    f"[bold red]Error: JSON file {file_path} did not load as a dictionary.[/]"
                )
                return None
            else:
                console.print(
                    f"[bold red]Error: Unsupported configuration file format: {file_extension}[/]"
                )
                return None
    except FileNotFoundError:
        console.print(f"[bold red]Error: Configuration file not found: {file_path}[/]")
        return None
    except tomllib.TOMLDecodeError as e:
        console.print(f"[bold red]Error decoding TOML file {file_path}: {e}[/]")
        return None
    except yaml.YAMLError as e:
        console.print(f"[bold red]Error decoding YAML file {file_path}: {e}[/]")
        return None
    except json.JSONDecodeError as e:
        console.print(f"[bold red]Error decoding JSON file {file_path}: {e}[/]")
        return None
    except Exception as e:
        console.print(f"[bold red]Error loading configuration file {file_path}: {e}[/]")
        return None
