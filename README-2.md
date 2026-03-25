# AWS FinOps Dashboard Executive Reporting Guide

This companion guide covers the executive PDF mode, Kubernetes cost integration, and the expanded report pipeline. It does not replace the main [README](./README.md); it focuses on the newer reporting features.

## Executive Report Mode

The CLI supports two PDF styles:

- `legacy`: the original PDF output
- `executive`: enhanced layout with cover page, KPI summary, charts, insights, and appendix

### Examples

```bash
# Standard PDF report
aws-finops --all --report-name monthly_report --report-type pdf

# Executive PDF report
aws-finops --all --report-name monthly_exec_report --report-type pdf --pdf-style executive

# Executive PDF report with OpenCost-backed Kubernetes data
aws-finops --all --report-name monthly_exec_k8s --report-type pdf --pdf-style executive --include-k8s --opencost-url http://localhost:9003
```

### Executive PDF Flags

| Flag | Description |
|---|---|
| `--pdf-style` | Selects `legacy` or `executive` PDF output. |
| `--pdf-logo-path` | Optional logo image for the executive report cover page. |
| `--pdf-confidentiality` | Optional confidentiality text for the executive report cover page. |
| `--pdf-chart-paths` | Optional pre-generated chart PNG paths to embed in the executive report. |

## Kubernetes Support

Kubernetes cost data is optional and uses OpenCost as an external source.

### Kubernetes Flags

| Flag | Description |
|---|---|
| `--include-k8s` | Enables Kubernetes cost collection for dashboard JSON and executive PDF output. |
| `--opencost-url` | Base URL for the OpenCost API. Default: `http://localhost:9003` |

When OpenCost is available, the dashboard can include:

- total cluster cost
- top namespaces by cost
- top workloads by cost
- idle cost
- shared cost
- unallocated cost

If OpenCost is not reachable, the AWS report still completes and the CLI prints a warning.

## Architecture

### AWS data collection

- The CLI discovers AWS profiles and regions.
- Cost Explorer, Budgets, and EC2 inventory data are collected per profile or combined account.
- The raw results are normalized into internal report models used by exports and renderers.

### OpenCost integration

- OpenCost is queried only when `--include-k8s` is enabled.
- The OpenCost adapter fetches allocation views for cluster, namespace, and workload cost data.
- Kubernetes results are normalized into internal Kubernetes report models so they can be exported consistently in JSON and executive PDF outputs.

### PDF rendering pipeline

- The export flow builds normalized report models first.
- Executive chart generation creates temporary PNG assets before PDF rendering.
- Structured insights are generated from AWS and optional Kubernetes data.
- The ReportLab renderer then builds either the legacy PDF or the executive PDF document from the normalized models, chart assets, and insights.

## Output Summary

### Terminal

- main AWS dashboard table
- executive insights summary
- OpenCost warnings when Kubernetes data is requested but partial or unavailable

### JSON

- AWS dashboard payload
- optional Kubernetes section when `--include-k8s` is enabled and data is available

### Executive PDF

- cover page
- KPI summary cards
- charts
- AWS cost breakdown sections
- optional Kubernetes cost section
- structured insights
- appendix
