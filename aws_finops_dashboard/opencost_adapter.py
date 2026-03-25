"""OpenCost adapter for optional Kubernetes cost retrieval."""

from typing import Any, Dict, List, Optional, Sequence, Tuple

import requests

from aws_finops_dashboard.report_models import KubernetesCostData, KubernetesCostItem


class OpenCostAdapter:
    """Fetch and normalize Kubernetes cost data from OpenCost."""

    def __init__(self, base_url: str, timeout: int = 5):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def fetch_costs(self, window: str) -> KubernetesCostData:
        """Fetch total, namespace, and workload costs for the requested window."""
        warnings: List[str] = []

        cluster_payload = self._get_allocation(window, aggregate="cluster", warnings=warnings)
        namespace_payload = self._get_allocation(
            window,
            aggregate="namespace",
            warnings=warnings,
        )
        workload_payload = self._get_allocation(
            window,
            aggregate="controller",
            warnings=warnings,
        )

        cluster_entries = _extract_allocation_entries(cluster_payload)
        namespace_entries = _extract_allocation_entries(namespace_payload)
        workload_entries = _extract_allocation_entries(workload_payload)

        total_cost = _sum_regular_costs(cluster_entries)
        if total_cost <= 0:
            total_cost = _sum_regular_costs(namespace_entries)

        namespace_costs = _to_ranked_items(namespace_entries)
        workload_costs = _to_ranked_items(workload_entries)

        idle_cost = _special_cost(cluster_entries, "__idle__")
        shared_cost = _special_cost(cluster_entries, "__shared__")
        unallocated_cost = _special_cost(cluster_entries, "__unallocated__")

        if not cluster_entries and not namespace_entries and not workload_entries:
            warnings.append("OpenCost returned no allocation data for the selected period.")

        return KubernetesCostData(
            source=self.base_url,
            window=window,
            total_cost=total_cost,
            namespace_costs=namespace_costs,
            workload_costs=workload_costs,
            idle_cost=idle_cost,
            shared_cost=shared_cost,
            unallocated_cost=unallocated_cost,
            warnings=warnings or None,
        )

    def _get_allocation(
        self,
        window: str,
        aggregate: str,
        warnings: List[str],
    ) -> Optional[Dict[str, Any]]:
        try:
            response = requests.get(
                f"{self.base_url}/allocation",
                params={
                    "window": window,
                    "aggregate": aggregate,
                    "includeIdle": "true",
                    "shareIdle": "false",
                },
                timeout=self.timeout,
            )
            response.raise_for_status()
            payload = response.json()
            if not isinstance(payload, dict):
                warnings.append(
                    f"OpenCost returned an unexpected payload for aggregate={aggregate}."
                )
                return None
            return payload
        except Exception as exc:
            warnings.append(
                f"OpenCost request failed for aggregate={aggregate}: {str(exc)}"
            )
            return None


def _extract_allocation_entries(
    payload: Optional[Dict[str, Any]],
) -> List[Tuple[str, Dict[str, Any]]]:
    if not payload:
        return []
    data = payload.get("data")
    if isinstance(data, list):
        merged_entries: List[Tuple[str, Dict[str, Any]]] = []
        for item in data:
            if isinstance(item, dict):
                merged_entries.extend(
                    [(key, value) for key, value in item.items() if isinstance(value, dict)]
                )
        return merged_entries
    if isinstance(data, dict):
        return [(key, value) for key, value in data.items() if isinstance(value, dict)]
    return []


def _to_ranked_items(entries: Sequence[Tuple[str, Dict[str, Any]]]) -> List[KubernetesCostItem]:
    items = []
    for key, value in entries:
        if key.startswith("__"):
            continue
        total_cost = _cost_from_entry(value)
        if total_cost > 0:
            items.append(KubernetesCostItem(name=key, cost=total_cost))
    items.sort(key=lambda item: item.cost, reverse=True)
    return items[:10]


def _sum_regular_costs(entries: Sequence[Tuple[str, Dict[str, Any]]]) -> float:
    return sum(
        _cost_from_entry(value)
        for key, value in entries
        if not key.startswith("__")
    )


def _special_cost(
    entries: Sequence[Tuple[str, Dict[str, Any]]],
    special_key: str,
) -> Optional[float]:
    for key, value in entries:
        if key == special_key:
            return _cost_from_entry(value)
    return None


def _cost_from_entry(entry: Dict[str, Any]) -> float:
    for key in ("totalCost", "cost", "total"):
        value = entry.get(key)
        if value is None:
            continue
        try:
            return float(value)
        except (TypeError, ValueError):
            continue
    return 0.0
