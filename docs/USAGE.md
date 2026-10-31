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

Live views select runs with `queued`, `in_progress`, `waiting`, `pending`, and `requested` statuses. Job-based views fetch jobs from the observed run attempt. The `runs` view reads only run metadata unless label/type filters require jobs. Individual job rows show only active jobs; aggregate JSON snapshots retain completed jobs within those active runs as activity context. Completed runs are not scanned by live views.

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
no_color: false
no_progress: false
details: false
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
| `--json` | `GHAQUEUE_JSON` | `false` | Emit JSON instead of the terminal report |
| `--no-color` | `GHAQUEUE_NO_COLOR` | `false` | Disable colored output |
| `--no-progress` | `GHAQUEUE_NO_PROGRESS` | `false` | Disable live scan progress |
| `--details` | `GHAQUEUE_DETAILS` | `false` | Show GitHub URLs and collection metadata |
| `--version` | `GHAQUEUE_VERSION` | `false` | Print build version and exit |

`--help` prints available flags. Durations use Go syntax: `30m`, `24h`, or `168h`; `7d` is not supported. `--since` affects only `failures`. Each HTTP request also has a 30-second timeout. API calls are read-only, paginated at 100 items per page, and not retried automatically.

Status applies to runs in `runs` and `failures`, and to jobs in `jobs`, `workflows`, and `pools`. In `overview`, run counts use run status and job totals use job status. Label/type filters select job demand; overview run counts remain the workflow-filtered run context. Runner inventory is filtered by label/type; workflow and status filters affect its observed job assignments, not inventory rows. Inventory itself reports `online`/`offline` and `busy`.

Jobs explicitly labeled `self-hosted` are classified as such. All other jobs have type `unknown`. Use `--label ubuntu-latest` for a hosted-image label; v1 does not claim that an arbitrary label proves a hosting type. Custom and scale-set labels may omit `self-hosted`.

## API Request Cost

Run discovery uses an adaptive strategy: read the complete history when there are at most 500 runs (one to five pages); otherwise use the five active-status queries after the initial probe. This saves calls for small histories while retaining old queued runs in larger histories. Larger histories cost six calls before additional pages and jobs. See [API Limitations](LIMITATIONS.md#visibility-pagination-and-cost).

For a lighter run-level view, `ghaqueue runs --status queued` makes one run-list request per repository before pagination and does not fetch jobs. This shows queued **runs**; use the job-based views to find queued **jobs** inside running workflows.

## Terminal Output and Progress

Terminal output uses Lip Gloss styles and a Bubbles progress bar, following the compact headings, indented results, and scan counter used by ghprmerge. It is a normal CLI: it does not start a TUI, read keyboard input, enter raw mode, or use an alternate screen.

Color is enabled when the destination is a terminal. A nonempty standard `NO_COLOR` environment variable, `--no-color`, or `TERM=dumb` disables color. `NO_COLOR` takes priority over `--no-color=false`. Redirected output is plain text, regardless of force-color environment settings.

During collection, progress is written to stderr only when both stdout and stderr are terminals. Discovery shows the current phase and elapsed time. Once repositories are discovered, the scan displays the completed/total repository count, a green bar, and a percentage. Workers report completion to one coordinator; progress updates never interleave with the final report. A repository counts as processed even when its API read fails; the final report marks partial results prominently. The percentage measures repository scanning, not workflow execution or queue drain.

```text
  Scanning  15/25 [█████████████████████████████████                      ]  60%
```

`--no-progress` disables the bar independently of color. `NO_COLOR` leaves progress enabled on terminals, using plain block characters. JSON mode, redirected streams, and `TERM=dumb` suppress progress entirely. Slow repository reads can leave the count unchanged while the scan continues.

## Reading the Output

The overview starts with pending and running job counts and the oldest available pending-job age. Runner demand comes next, followed by repository demand. It shows at most five label groups and eight repositories; focused `pools`, `workflows`, and `jobs` views show all matching active work. Groups with only completed jobs are omitted. Counts for the number of repositories in a group refer to matching active jobs.

Runs with no visible jobs and waiting runs are called out separately, since job totals cannot describe them. Use `runs` to inspect these workflows. Partial-snapshot warnings appear before totals so that incomplete data is visible immediately.

Pending includes all observed job statuses other than `in_progress` and `completed`. It does not imply that a job is ready for a runner.

- **Oldest pending / pending age:** elapsed time since job `created_at`; `unknown` when absent. This includes time before a job becomes eligible to run.
- **Running-job age:** elapsed time since job `started_at`.
- **Run age:** elapsed time since workflow run creation, separate from job wait age. A rerun retains the original run creation timestamp.

Ages use compact minute/hour/day values. Jobs appear in pending and running sections, oldest first within each section; missing ages sort last. Workflow runs appear oldest first, while recent failures show the most recently updated first. Offline runners appear first, followed by busy runners.

Demand groups sort by pending count, running count, then name. Workflow groups combine exact workflow names across repositories. Label pools combine sorted exact label sets. Narrow terminals use stacked group rows and wrap item text rather than producing oversized tables.

`--details` adds GitHub links to individual jobs and runs, plus the observation time, collection duration, and API request count. The JSON snapshot always retains URLs and raw timestamps regardless of this flag. Job start/completion timestamps remain available in JSON, including completed jobs within active runs; the default display focuses on current demand rather than historical activity context.

## JSON and Exit Codes

JSON contains `started_at`, `finished_at`, `repositories`, `api_requests`, selected `runs` with nested jobs, optional `runners`, and `warnings`. Aggregate commands emit the underlying observations, allowing scripts to choose their own grouping. `repositories` counts repositories selected for scanning, including those whose reads failed. Field timestamps come directly from GitHub; absent timestamps may be represented by Go's zero timestamp. No durations or trend estimates are persisted. `api_requests` counts attempted HTTP calls, including failed calls and pagination, and excludes authentication lookup through `gh`. `runs` snapshots omit nested jobs unless a label/type filter required job reads.

| Exit code | Meaning |
|-----------|---------|
| `0` | Completed scan, including an empty result; help/version also succeed |
| `1` | Invalid configuration, output failure, or repository discovery failure |
| `2` | Partial snapshot: one or more reads failed or a run query reached its result limit |

Warnings go to stderr and are also included in JSON. Always inspect the exit code before interpreting zero counts. A `403` may indicate missing permissions or rate limiting; rate-limit response headers are included in errors when available.
