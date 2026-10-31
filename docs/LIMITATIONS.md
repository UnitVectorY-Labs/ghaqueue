---
layout: default
title: API Limitations
nav_order: 5
permalink: /limitations
---

# API Limitations
{: .no_toc }

## Table of Contents
{: .no_toc .text-delta }

- TOC
{:toc}

## What a Snapshot Can Establish

A snapshot establishes which work GitHub returned during the collection interval, its reported status, requested labels, timestamps, and assigned runners. It can reveal a large backlog, old pending jobs, offline runners, busy runners, and running jobs competing for the same labels.

It cannot establish drain rate, a trend, an estimated completion time, or a definitive stall without observations over time. A pool with no running jobs and old pending work deserves investigation; that combination alone does not prove that capacity is unavailable. Job start/finish timestamps in JSON are activity clues within the active runs scanned, not evidence that the entire queue is advancing. No historical state is stored.

## Runs and Jobs Are Different

Workflow status and job status need not agree. An in-progress workflow can contain queued jobs and completed jobs. A waiting workflow can be blocked by environment approval or other conditions. Dependencies and concurrency controls can delay jobs without any runner shortage.

Jobs may not exist in the API yet. Run totals therefore remain separate from job totals. Run age is not runner queue duration. Job creation age is also an approximation of waiting: GitHub does not expose a universal timestamp for when each job became eligible for a runner. Missing job timestamps remain `unknown`; no wait timestamp is fabricated from the workflow's creation date.

Job reads are pinned to the run attempt observed during collection. A workflow can finish or be rerun during the scan. The result is not an atomic organization snapshot, and pagination can shift while new work arrives. Runs returned by multiple status queries are deduplicated per repository.

See the [workflow runs API](https://docs.github.com/en/rest/actions/workflow-runs) and [workflow jobs API](https://docs.github.com/en/rest/actions/workflow-jobs).

## Labels Describe Demand, Not Guaranteed Capacity

Pool groups use exact sorted job label sets across repositories. These groups are useful for identifying shared demand, but they are not disjoint capacity pools. One runner may satisfy several sets. Runner groups, repository authorization, scale sets, autoscaling, and custom scheduling can further constrain access.

`self-hosted` labels identify explicitly labeled self-hosted jobs. Other labels remain type `unknown`, including familiar hosted-image labels: arbitrary custom labels and scale sets prevent a reliable universal hosting classification from labels alone. Filter by the actual label when that is the useful distinction.

GitHub-hosted runner fleet capacity, organization concurrency allowance, queue position, and the exact reason a job is waiting are not provided by the APIs used here. The CLI does not invent utilization percentages or a hosted runner inventory.

## Runner Inventory

Only `runners` reads self-hosted inventory. Organization selection reads organization inventory and each selected repository's runner inventory. Repository selection also attempts the owner's organization inventory; personal-account owners or insufficient organization permissions can produce a partial-result warning. Repository and organization scopes remain explicit rather than being added together as a capacity total.

A busy runner can be executing work outside the selected repositories. Missing observed assignments do not mean that the runner is idle. Seeing an online idle runner does not prove a pending job can use it. Organization runner inventory does not establish runner-group access for each repository. Enterprise-level inventory and GitHub-hosted fleets are not enumerated.

See [self-hosted runner API access requirements](https://docs.github.com/en/rest/actions/self-hosted-runners).

## Visibility, Pagination, and Cost

Only repositories visible to the authenticated caller can be scanned. A successful organization listing does not prove the token can see every private repository. Archived and disabled repositories are excluded from organization discovery. Restricted inventory access yields warnings, not invented zero capacity.

GitHub limits filtered workflow run queries to 1,000 results per repository/query. When using status-specific queries, ghaqueue reads up to that limit for each active status or failure conclusion, emits a warning when more results may exist, and returns exit code `2`. Repository, job, and runner listings otherwise continue through their pages. Narrowing the repository scope can reduce cost; workflow and label filters are applied locally and do not bypass a per-repository run-query limit.

Live scans first fetch one unfiltered page of up to 100 runs per repository. If GitHub reports at most 500 stored runs, ghaqueue reads the complete history in one to five requests and selects active runs locally. If more than 500 runs exist, it switches to the five active-status queries rather than assuming that the latest page contains all pending work. A history that grows past the threshold during pagination also falls back to status queries. This remains stateless and imposes no age cutoff on queued work.

For histories of up to 100 runs, that reduces run-discovery calls from five to one. For histories over 500, the probe adds one call: six before pagination and job reads. The optimization saves requests in organizations with many small histories; it does not promise a reduction for every organization. GitHub's run-list API documents one status filter at a time, without an all-active or exclude-completed filter.

The `runs` view skips job reads unless label/type filtering requires them. An explicit `runs --status` selects that status at the API rather than scanning all five. Job-based views still fetch jobs for all observed active runs, including in-progress runs that can contain queued jobs. Unused pull-request details are excluded from run responses to reduce payload size. `--details` and JSON report the number of API requests attempted by the scan.

Large organizations can still require hundreds or thousands of requests. Repository concurrency defaults to eight; tune `--workers` and `--timeout` to fit the organization and token limits. Failed requests are not retried. Warnings identify affected reads and include rate-limit headers where present. Never treat a partial snapshot as a complete count.

## Recent Failures

The failures view queries current run attempts with `failure`, `timed_out`, or `action_required` conclusions, then filters GitHub's `updated_at` against `--since`. This is a recent-update window, not an exact failure-time window. Successfully retried attempts are no longer returned as failures, and cancelled, stale, neutral, and skipped conclusions are excluded.

Queries scan the available failure history because an old run can be rerun or updated recently. The 1,000-result limit still applies independently to each conclusion. A long history can therefore produce partial results even for a short requested window. Results link to GitHub for investigation; retrying, cancelling, and modifying workflows are outside v1.
