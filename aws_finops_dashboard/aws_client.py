from collections import defaultdict
from typing import List, Optional

import boto3
from boto3.session import Session
from rich.console import Console

from aws_finops_dashboard.types import EC2Summary, RegionName

console = Console()


def get_aws_profiles() -> List[str]:
    """Get all configured AWS profiles from the AWS CLI configuration."""
    try:
        session = boto3.Session()
        return session.available_profiles
    except Exception as e:
        console.print(f"[bold red]Error getting AWS profiles: {str(e)}[/]")
        return []


def get_account_id(session: Session) -> Optional[str]:
    """Get the AWS account ID for a session."""
    try:
        account_id = session.client("sts").get_caller_identity().get("Account")
        return str(account_id) if account_id is not None else None
    except Exception as e:
        console.log(f"[yellow]Warning: Could not get account ID: {str(e)}[/]")
        return None


def get_all_regions(session: Session) -> List[RegionName]:
    """
    Get all available AWS regions.
    Using us-east-1 as a default region to get the list of all regions.

    If the call fails, it will return a hardcoded list of common regions.
    """
    try:
        ec2_client = session.client("ec2", region_name="us-east-1")
        regions = [
            region["RegionName"] for region in ec2_client.describe_regions()["Regions"]
        ]
        return regions
    except Exception as e:
        console.log(f"[yellow]Warning: Could not get all regions: {str(e)}[/]")
        return [
            "us-east-1",
            "us-east-2",
            "us-west-1",
            "us-west-2",
            "ap-southeast-1",
            "ap-south-1",
            "eu-west-1",
            "eu-west-2",
            "eu-central-1",
        ]


def get_accessible_regions(session: Session) -> List[RegionName]:
    """Get regions that are accessible with the current credentials."""
    all_regions = get_all_regions(session)
    accessible_regions = []

    for region in all_regions:
        try:
            ec2_client = session.client("ec2", region_name=region)
            ec2_client.describe_instances(MaxResults=5)
            accessible_regions.append(region)
        except Exception:
            console.log(
                f"[yellow]Region {region} is not accessible with the current credentials[/]"
            )

    if not accessible_regions:
        console.log("[yellow]No accessible regions found. Using default regions.[/]")
        return ["us-east-1", "us-east-2", "us-west-1", "us-west-2"]

    return accessible_regions


def ec2_summary(
    session: Session, regions: Optional[List[RegionName]] = None
) -> EC2Summary:
    """Get EC2 instance summary across specified regions or all regions."""
    if regions is None:
        regions = [
            "us-east-1",
            "us-east-2",
            "us-west-1",
            "us-west-2",
            "ap-southeast-1",
            "ap-south-1",
            "eu-central-1",
            "eu-west-1",
            "eu-west-2",
        ]

    instance_summary: EC2Summary = defaultdict(int)

    for region in regions:
        try:
            ec2_regional = session.client("ec2", region_name=region)
            instances = ec2_regional.describe_instances()
            for reservation in instances["Reservations"]:
                for instance in reservation["Instances"]:
                    state = instance["State"]["Name"]
                    instance_summary[state] += 1
        except Exception as e:
            console.log(
                f"[yellow]Warning: Could not access EC2 in region {region}: {str(e)}[/]"
            )

    if "running" not in instance_summary:
        instance_summary["running"] = 0
    if "stopped" not in instance_summary:
        instance_summary["stopped"] = 0

    return instance_summary


def get_inactive_ec2_instances(session: Session, regions: List[str]) -> List[str]:
    """Returns a list of stopped EC2 instances in the specified regions."""
    inactive_instances = []
    for region in regions:
        ec2_client = session.client("ec2", region_name=region)
        response = ec2_client.describe_instances(
            Filters=[{"Name": "instance-state-name", "Values": ["stopped"]}]
        )
        for reservation in response["Reservations"]:
            for instance in reservation["Instances"]:
                inactive_instances.append(instance["InstanceId"])
    return inactive_instances


def get_unused_ebs_volumes(session: Session, regions: List[str]) -> List[str]:
    """Returns a list of unattached EBS volumes in the specified regions."""
    unused_volumes = []
    for region in regions:
        ec2_client = session.client("ec2", region_name=region)
        response = ec2_client.describe_volumes(
            Filters=[{"Name": "status", "Values": ["available"]}]
        )
        for volume in response["Volumes"]:
            unused_volumes.append(volume["VolumeId"])
    return unused_volumes
