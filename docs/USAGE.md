---
layout: default
title: Usage
nav_order: 3
permalink: /usage
---

# Usage
{: .no_toc }

## Table of Contents
{: .no_toc .text-delta }

- TOC
{:toc}

## Commands

```text
ghaqueue [overview|jobs|runs|workflows|pools|runners|failures] [flags]
```

Flags can appear before or after the command. Omitting the command selects `overview`.

| Command | Operational question |
|---------|----------------------|
| `overview` | How much work is pending or running, and in which repositories? |
| `jobs` | Which individual jobs are pending or running, on which labels and runners? |
| `runs` | Which active workflow runs exist, including runs with no jobs visible yet? |
| `workflows` | Which common workflow names account for job demand across repositories? |
| `pools` | Which exact runner label sets have the most pending jobs? |
| `runners` | Which visible self-hosted runners are online, busy, or serving observed jobs? |
| `failures` | Which current run attempts have failure, timed-out, or action-required conclusions and were updated recently? |

Live views fetch runs with `queued`, `in_progress`, `waiting`, `pending`, and `requested` statuses. Jobs come from the observed run attempt. Individual job rows show only active jobs; aggregate views include completed jobs within those active runs as activity context. Completed runs are not scanned by live views.

## Repository and Organization Selection

Explicit `--repo` or `--org` selects the scope. Otherwise, nonempty `GHAQUEUE_REPO` or `GHAQUEUE_ORG` selects it. An explicit organization selection clears a repository inherited from the configuration file unless a repository is also selected explicitly or through the environment.

Without an explicit scope:

1. Use the current git repository when its `origin` points to `github.com`.
2. Use defaults from `.ghaqueue` in the current directory or nearest ancestor.
3. If no scope is available, report an error explaining how to select one.

The organization listing includes accessible, nonarchived, nondisabled repositories. `--repo OWNER/REPO` selects one repository directly, including archived repositories. `--repo name --org organization` is shorthand for `organization/name`. GitHub HTTPS and SSH remotes are recognized. The inferred scope always uses `origin`.

Configuration defaults are loaded from the nearest `.ghaqueue`; flags override environment values, which override file values. Repository inference overrides only file scope defaults. Filters and other settings from the file still apply inside a repository. Invalid or unknown configuration fields are errors. An empty string environment value clears that file setting; empty workers/boolean values are errors. Boolean flags accept `--json=false`.

## Configuration File

`.ghaqueue` is optional YAML:

```yaml
organization: UnitVectorY-Labs
workers: 8
timeout: 5m
since: 24h
# repo: project-name
# workflow: build
# label: self-hosted
# runner_type: self-hosted
# status: queued
json: false
```

Do not store tokens in this file. Use authentication environment variables or `gh`.

## Flags and Environment Variables

| Flag | Environment variable | Default | Purpose |
|------|----------------------|---------|---------|
| `--org` | `GHAQUEUE_ORG` | inferred/file | Organization to scan |
| `--repo` | `GHAQUEUE_REPO` | inferred/file | `OWNER/REPO`, or name with organization |
| `--workflow` | `GHAQUEUE_WORKFLOW` | all | Case-insensitive substring of workflow name or path |
| `--label` | `GHAQUEUE_LABEL` | all | Case-insensitive exact job runner label |
| `--runner-type` | `GHAQUEUE_RUNNER_TYPE` | all | `self-hosted` or `unknown` |
| `--status` | `GHAQUEUE_STATUS` | all | `queued`, `in_progress`, `waiting`, `pending`, `requested`, or `completed` |
| `--since` | `GHAQUEUE_SINCE` | `24h` | Failure window, measured against run `updated_at` |
| `--workers` | `GHAQUEUE_WORKERS` | `8` | Concurrent repositories, 1–64 |
| `--timeout` | `GHAQUEUE_TIMEOUT` | `5m` | Overall collection deadline |
| `--json` | `GHAQUEUE_JSON` | `false` | Emit JSON instead of terminal tables |
| `--version` | `GHAQUEUE_VERSION` | `false` | Print build version and exit |

`--help` prints available flags. Durations use Go syntax: `30m`, `24h`, or `168h`; `7d` is not supported. `--since` affects only `failures`. Each HTTP request also has a 30-second timeout. API calls are read-only, paginated at 100 items per page, and not retried automatically.

Status applies to runs in `runs` and `failures`, and to jobs in `jobs`, `workflows`, and `pools`. In `overview`, run counts use run status and job totals use job status. Label/type filters select job demand; overview run counts remain the workflow-filtered run context. Runner inventory is filtered by label/type; workflow and status filters affect its observed job assignments, not inventory rows. Inventory itself reports `online`/`offline` and `busy`.

Jobs explicitly labeled `self-hosted` are classified as such. All other jobs have type `unknown`. Use `--label ubuntu-latest` for a hosted-image label; v1 does not claim that an arbitrary label proves a hosting type. Custom and scale-set labels may omit `self-hosted`.

## Reading the Output

Pending includes all observed job statuses other than `in_progress` and `completed`. It does not imply that a job is ready for a runner.

- **Oldest pending / wait age:** elapsed time since job `created_at`; `unknown` when absent. This includes time before a job becomes eligible to run.
- **Run age:** elapsed time since workflow run creation, separate from job wait age. A rerun retains the original run creation timestamp.
- **Run age for jobs in progress:** the jobs view's `WAIT / RUN AGE` column switches to elapsed time since job `started_at`.
- **Last start / finish:** age of the latest job timestamp found within collected active runs. Completed runs are outside this context.
- **Done:** completed jobs belonging to active runs, not completed workflows or historical throughput.

Groups sort by pending job count, then name. Workflow groups combine exact workflow names across repositories. Label pools combine sorted exact label sets. Individual runs sort oldest first; jobs retain GitHub's order within each run. Each snapshot reports collection start/end times in JSON and collection duration in terminal output. GitHub is changing during the scan, so these are observations across an interval.

## JSON and Exit Codes

JSON contains `started_at`, `finished_at`, `repositories`, selected `runs` with nested jobs, optional `runners`, and `warnings`. Aggregate commands emit the underlying observations, allowing scripts to choose their own grouping. `repositories` counts repositories selected for scanning, including those whose reads failed. Field timestamps come directly from GitHub; absent timestamps may be represented by Go's zero timestamp. No durations or trend estimates are persisted.

| Exit code | Meaning |
|-----------|---------|
| `0` | Completed scan, including an empty result; help/version also succeed |
| `1` | Invalid configuration, output failure, or repository discovery failure |
| `2` | Partial snapshot: one or more reads failed or a run query reached its result limit |

Warnings go to stderr and are also included in JSON. Always inspect the exit code before interpreting zero counts. A `403` may indicate missing permissions or rate limiting; rate-limit response headers are included in errors when available.
