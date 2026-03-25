import csv
import json
import os
import re
import shutil
import sys
from datetime import datetime
from io import BytesIO, StringIO
from typing import Any, Dict, List, Optional

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
from rich.console import Console

from aws_finops_dashboard.pdf_renderers import (
    PdfStyle,
    render_audit_report_pdf,
    render_cost_dashboard_pdf,
)
from aws_finops_dashboard.report_models import (
    ChartAsset,
    KubernetesCostData,
    build_dashboard_report,
)
from aws_finops_dashboard.types import ProfileData

console = Console()

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
    pdf_style: PdfStyle = "legacy",
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

        render_audit_report_pdf(
            pdf_output,
            audit_data_list=audit_data_list,
            pdf_style=pdf_style,
        )

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
    pdf_style: PdfStyle = "legacy",
    previous_period_name: str = "Previous Period",
    current_period_name: str = "Current Period",
    logo_path: Optional[str] = None,
    confidentiality_notice: Optional[str] = None,
    chart_image_paths: Optional[List[str]] = None,
    kubernetes_costs: Optional[KubernetesCostData] = None,
) -> Optional[str]:
    from aws_finops_dashboard.export_handler import ExportHandler
    from aws_finops_dashboard.executive_chart_generator import (
        generate_executive_chart_bundle,
    )

    generated_chart_dir: Optional[str] = None
    try:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M")
        base_filename = f"{filename}_{timestamp}.pdf"

        # Use export handler if provided, otherwise create default
        if export_handler is None:
            export_handler = ExportHandler(local_dir=output_dir)

        # Get output destination (BytesIO for S3, file path for local)
        pdf_output = export_handler.get_pdf_output(base_filename)

        resolved_chart_paths = chart_image_paths
        resolved_chart_assets = None
        if pdf_style == "executive":
            report = build_dashboard_report(
                data,
                previous_period_dates=previous_period_dates,
                current_period_dates=current_period_dates,
                previous_period_name=previous_period_name,
                current_period_name=current_period_name,
                logo_path=logo_path,
                confidentiality_notice=confidentiality_notice,
                chart_image_paths=None,
                kubernetes_costs=kubernetes_costs,
            )
            generated_bundle = generate_executive_chart_bundle(report)
            generated_chart_dir = generated_bundle.temp_dir
            resolved_chart_assets = list(generated_bundle.chart_assets)
            if chart_image_paths:
                resolved_chart_assets.extend(
                    [
                        ChartAsset(title=f"Chart {index + 1}", path=path)
                        for index, path in enumerate(chart_image_paths)
                    ]
                )
                resolved_chart_paths = [
                    asset.path for asset in resolved_chart_assets if asset.path
                ]
            else:
                resolved_chart_paths = [
                    asset.path for asset in resolved_chart_assets if asset.path
                ]

        render_cost_dashboard_pdf(
            pdf_output,
            data=data,
            previous_period_dates=previous_period_dates,
            current_period_dates=current_period_dates,
            pdf_style=pdf_style,
            previous_period_name=previous_period_name,
            current_period_name=current_period_name,
            logo_path=logo_path,
            confidentiality_notice=confidentiality_notice,
            chart_image_paths=resolved_chart_paths,
            chart_assets=resolved_chart_assets,
            kubernetes_costs=kubernetes_costs,
        )

        # Finalize PDF export
        return export_handler.finalize_pdf(pdf_output, base_filename)
    except Exception as e:
        console.print(f"[bold red]Error exporting to PDF: {str(e)}[/]")
        return None
    finally:
        if generated_chart_dir:
            shutil.rmtree(generated_chart_dir, ignore_errors=True)


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
