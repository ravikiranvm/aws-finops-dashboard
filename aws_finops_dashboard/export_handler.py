"""Export handler for managing export destinations (local file, S3, or Slack)."""
import os
from datetime import datetime
from typing import Optional

from boto3.session import Session
from botocore.exceptions import ClientError
from rich.console import Console

from aws_finops_dashboard.helpers import upload_to_s3

console = Console()


def generate_slack_message(
    report_type: str,
    report_name: str,
    profiles: Optional[list] = None,
    time_period: Optional[str] = None,
) -> str:
    """
    Generate a contextual message for Slack file upload.

    Args:
        report_type: Type of report (dashboard, audit, trend)
        report_name: Name of the report
        profiles: List of AWS profiles used
        time_period: Time period description (e.g., "Current Month", "Last 30 days")

    Returns:
        Formatted message string
    """
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    
    if report_type == "dashboard":
        profile_info = f"Profiles: {', '.join(profiles)}" if profiles else "All profiles"
        period_info = f"Period: {time_period}" if time_period else ""
        message = f"📊 *A new cost & audit report from the AWS FinOps Dashboard is available*\n\n"
        message += f"Report: {report_name}\n"
        message += f"{profile_info}\n"
        if period_info:
            message += f"{period_info}\n"
        message += f"Generated on: {timestamp}"
        
    elif report_type == "audit":
        profile_info = f"Profiles: {', '.join(profiles)}" if profiles else "All profiles"
        message = f"📊 *A new audit report from the AWS FinOps Dashboard is available*\n\n"
        message += f"Report: {report_name}\n"
        message += f"{profile_info}\n"
        message += f"Generated on: {timestamp}\n\n"
        
    elif report_type == "trend":
        profile_info = f"Profiles: {', '.join(profiles)}" if profiles else "All profiles"
        message = f"📈 *A new cost trend analysis report from the AWS FinOps Dashboard is available*\n\n"
        message += f"Report: {report_name}\n"
        message += f"{profile_info}\n"
        message += f"Period: Last 6 months\n"
        message += f"Generated on: {timestamp}"
        
    else:
        message = f"📊 *A new report from the AWS FinOps Dashboard is available*\n\n"
        message += f"Report: {report_name}\n"
        message += f"Generated on: {timestamp}"
    
    return message


class ExportHandler:
    """Handles export destination (local file, S3, or Slack)."""

    def __init__(
        self,
        s3_bucket: Optional[str] = None,
        s3_prefix: Optional[str] = None,
        session: Optional[Session] = None,
        local_dir: Optional[str] = None,
        slack_token: Optional[str] = None,
        slack_channel: Optional[str] = None,
        slack_message: Optional[str] = None,
    ):
        """
        Initialize export handler.

        Args:
            s3_bucket: S3 bucket name for S3 exports
            s3_prefix: S3 key prefix/folder path
            session: Boto3 session for S3 uploads
            local_dir: Local directory for file exports
            slack_token: Slack bot token for Slack exports
            slack_channel: Slack channel/user identifier for Slack exports
            slack_message: Message to include with Slack file upload
        """
        self.s3_bucket = s3_bucket
        self.s3_prefix = s3_prefix
        self.session = session
        self.local_dir = local_dir
        self.slack_token = slack_token
        self.slack_channel = slack_channel
        self.slack_message = slack_message
        self.use_slack = bool(slack_token and slack_channel)
        self.use_s3 = bool(s3_bucket and session)

    def save(
        self,
        content: bytes,
        filename: str,
        content_type: Optional[str] = None,
    ) -> Optional[str]:
        """
        Save content to local file, S3, or Slack.

        Args:
            content: Content to save as bytes
            filename: Base filename (will have timestamp added)
            content_type: MIME type (auto-detected if not provided)

        Returns:
            Path to saved file (local path, S3 URI, or Slack file ID), or None on error
        """
        if self.use_slack:
            return self._save_to_slack(content, filename, content_type)
        elif self.use_s3:
            return self._save_to_s3(content, filename, content_type)
        else:
            return self._save_to_local(content, filename)

    def _save_to_s3(
        self, content: bytes, filename: str, content_type: Optional[str] = None
    ) -> Optional[str]:
        """Save content to S3."""
        try:
            # Build S3 key
            s3_key = f"{self.s3_prefix}/{filename}" if self.s3_prefix else filename
            s3_key = s3_key.lstrip("/")  # Remove leading slash

            # Auto-detect content type if not provided
            if not content_type:
                if filename.endswith(".pdf"):
                    content_type = "application/pdf"
                elif filename.endswith(".csv"):
                    content_type = "text/csv"
                elif filename.endswith(".json"):
                    content_type = "application/json"

            # Upload to S3
            s3_path = upload_to_s3(
                content, self.s3_bucket, s3_key, self.session, content_type
            )
            if s3_path:
                console.print(
                    f"[bright_green]Successfully exported to S3: {s3_path}[/]"
                )
            return s3_path
        except Exception as e:
            console.print(f"[bold red]Error saving to S3: {str(e)}[/]")
            return None

    def _save_to_slack(
        self, content: bytes, filename: str, content_type: Optional[str] = None
    ) -> Optional[str]:
        """Save content to Slack."""
        try:
            from slack_sdk import WebClient
            from slack_sdk.errors import SlackApiError
        except ImportError:
            console.print(
                "[bold red]Error: slack-sdk not installed. Please install it with: pip install slack-sdk[/]"
            )
            return None

        try:
            # Auto-detect content type if not provided
            if not content_type:
                if filename.endswith(".pdf"):
                    content_type = "application/pdf"
                elif filename.endswith(".csv"):
                    content_type = "text/csv"
                elif filename.endswith(".json"):
                    content_type = "application/json"

            # Initialize Slack client
            client = WebClient(token=self.slack_token)

            # Prepare upload parameters
            upload_params = {
                "channel": self.slack_channel,
                "file": content,
                "filename": filename,
                "title": filename,
            }

            # Add message/comment if provided
            if self.slack_message:
                upload_params["initial_comment"] = self.slack_message

            # Upload file to Slack
            response = client.files_upload_v2(**upload_params)

            file_id = response.get("file", {}).get("id")
            if file_id:
                console.print(
                    f"[bright_green]Successfully exported to Slack: {filename} (File ID: {file_id})[/]"
                )
            return file_id

        except SlackApiError as e:
            error_message = e.response.get("error", "Unknown error") if e.response else str(e)
            console.print(
                f"[bold red]Error uploading to Slack: {error_message}[/]"
            )
            return None
        except Exception as e:
            console.print(f"[bold red]Error saving to Slack: {str(e)}[/]")
            return None

    def _save_to_local(self, content: bytes, filename: str) -> Optional[str]:
        """Save content to local file."""
        try:
            output_filename = filename
            if self.local_dir:
                os.makedirs(self.local_dir, exist_ok=True)
                output_filename = os.path.join(self.local_dir, filename)
            else:
                output_filename = filename

            # Use text mode for CSV/JSON, binary for PDF
            if filename.endswith((".csv", ".json")):
                with open(output_filename, "w", encoding="utf-8", newline="" if filename.endswith(".csv") else "") as f:
                    f.write(content.decode("utf-8"))
            else:
                # Binary mode for PDF and other files
                with open(output_filename, "wb") as f:
                    f.write(content)

            return os.path.abspath(output_filename)
        except Exception as e:
            console.print(f"[bold red]Error saving to local file: {str(e)}[/]")
            return None

    def get_pdf_output(self, base_filename: str):
        """
        Get output destination for PDF generation.
        Returns BytesIO for S3/Slack, or file path for local.

        Args:
            base_filename: Filename to use for local exports

        Returns:
            BytesIO instance (if S3/Slack) or file path string (if local)
        """
        from io import BytesIO

        if self.use_s3 or self.use_slack:
            return BytesIO()
        else:
            if self.local_dir:
                os.makedirs(self.local_dir, exist_ok=True)
                return os.path.join(self.local_dir, base_filename)
            return base_filename

    def finalize_pdf(self, pdf_buffer_or_path, base_filename: str) -> Optional[str]:
        """
        Finalize PDF export - upload to S3/Slack or return local path.

        Args:
            pdf_buffer_or_path: BytesIO buffer (if S3/Slack) or file path (if local)
            base_filename: Final filename to use (for S3 key or Slack upload)

        Returns:
            S3 path, Slack file ID, or local file path
        """
        if self.use_slack:
            # pdf_buffer_or_path is BytesIO
            pdf_buffer_or_path.seek(0)
            pdf_content = pdf_buffer_or_path.getvalue()
            return self._save_to_slack(pdf_content, base_filename, "application/pdf")
        elif self.use_s3:
            # pdf_buffer_or_path is BytesIO
            pdf_buffer_or_path.seek(0)
            pdf_content = pdf_buffer_or_path.getvalue()
            return self._save_to_s3(pdf_content, base_filename, "application/pdf")
        else:
            # pdf_buffer_or_path is the file path, just return it
            return os.path.abspath(pdf_buffer_or_path)

