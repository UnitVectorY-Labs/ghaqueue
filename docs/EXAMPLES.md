---
layout: default
title: Examples
nav_order: 4
permalink: /examples
---

# Examples
{: .no_toc }

## Table of Contents
{: .no_toc .text-delta }

- TOC
{:toc}

## After a Burst of Merges

```bash
ghaqueue overview --org UnitVectorY-Labs
ghaqueue pools --org UnitVectorY-Labs
ghaqueue workflows --org UnitVectorY-Labs
```

Start with repository totals, then compare label demand and common workflow names across repositories. A pool with pending work and no running jobs is worth investigating. It does not establish a stall: jobs may be waiting for dependencies, concurrency groups, or approvals.

## Inspect a Shared Worker Pool

```bash
ghaqueue jobs --org UnitVectorY-Labs --label self-hosted --status queued
ghaqueue jobs --org UnitVectorY-Labs --label linux --status in_progress
ghaqueue runners --org UnitVectorY-Labs --label linux
```

Compare exact label sets and runner assignments. A runner must satisfy all requested labels and repository/group access rules. The inventory may require broader permissions than job visibility.

## Investigate One Workflow

```bash
ghaqueue runs --repo UnitVectorY-Labs/ghorgsync --workflow build --details
ghaqueue jobs --repo UnitVectorY-Labs/ghorgsync --workflow build --details
```

Use the run view to see workflows that have not materialized jobs yet. Open the emitted GitHub URL to inspect concurrency or approval conditions.

## Recent Failures

```bash
ghaqueue failures --org UnitVectorY-Labs --since 24h
ghaqueue failures --org UnitVectorY-Labs --since 168h --workflow build
```

The window applies to GitHub's last update timestamp, not an inferred failure timestamp. Results represent current failed run attempts; successfully retried runs disappear. The CLI supplies links for investigation and does not retry jobs.

## Outside a Repository

Create `.ghaqueue` in the folder where you normally work:

```yaml
organization: UnitVectorY-Labs
workers: 8
```

```bash
ghaqueue
ghaqueue pools
```

Inside a GitHub checkout, use `--org UnitVectorY-Labs` to override repository inference.

## Scripted Snapshots

```bash
GHAQUEUE_ORG=UnitVectorY-Labs GHAQUEUE_JSON=true ghaqueue jobs > snapshot.json
```

Check the command's exit code before consuming the file. Code `2` means that a usable but incomplete snapshot was written; inspect its `warnings`. The file is an explicit export, not state maintained by the application.

## Terminal Preferences

```bash
# Keep progress, remove color
NO_COLOR=1 ghaqueue --org UnitVectorY-Labs

# Suppress progress, keep terminal colors
ghaqueue --org UnitVectorY-Labs --no-progress

# Include GitHub links for investigation
ghaqueue jobs --org UnitVectorY-Labs --details
```

Piping or redirecting either stream disables progress automatically. Redirected report output contains no color codes. For CI, `--json` also suppresses progress.

## Example Overview

This illustrative snapshot scanned 100 repositories, found active jobs in two, and found one waiting workflow with no jobs visible:

```text
ghaqueue ─ acme
100 repositories scanned · overview

  2 pending  ·  1 running
  Active jobs across 2 repositories
  Oldest pending: 23m
  1 active workflow run has no jobs visible yet
  1 workflow run is waiting; check GitHub for approval or other gates

Runner demand
Runner labels            Pending  Running  Oldest pending  Repos
build,linux,self-hosted  2        0        23m             2
ubuntu-latest            0        1        —               1

Repositories
Repository  Pending  Running  Oldest pending
acme/api    1        1        23m
acme/web    1        0        18m

Pending age includes dependency and approval waits.

Inspect: jobs · runs · workflows · pools · runners
```

The label view highlights pending self-hosted demand separately from running work on `ubuntu-latest`. Zero running jobs for a label set does not establish a stall. Use `jobs --label self-hosted` and `runners` to investigate.
