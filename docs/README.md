---
layout: default
title: ghaqueue
nav_order: 1
permalink: /
---

# ghaqueue

Command-line observability for GitHub Actions queues across repositories, workflows, and runners.

**ghaqueue** shows the work pending and running across a GitHub organization. It brings together workflow runs, jobs, runner labels, and self-hosted runner inventory so that a burst of merges can be understood from the terminal.

## Key Features

- **Organization overview** — pending and running jobs grouped by repository
- **Individual work** — active jobs and workflow runs with ages and GitHub links
- **Shared demand** — aggregate common workflow names and runner label sets across repositories
- **Runner inventory** — self-hosted runner status, occupancy, and observed assignments
- **Recent failures** — find failed, timed out, and action-required runs for investigation
- **Stateless and read-only** — every invocation reads GitHub; no database, background process, or workflow mutations
- **Readable terminal output** — colored summaries, scan progress, and compact demand views; honors `NO_COLOR`
- **Scriptable output** — JSON snapshots and explicit exit codes for incomplete results

## Quick Start

```bash
# Inside a GitHub repository
ghaqueue

# Across an organization
ghaqueue --org UnitVectorY-Labs
ghaqueue pools --org UnitVectorY-Labs
ghaqueue jobs --org UnitVectorY-Labs --label self-hosted

# Investigate recent failures
ghaqueue failures --org UnitVectorY-Labs --since 24h
```

See [Installation](INSTALL.md), [Usage](USAGE.md), [Examples](EXAMPLES.md), and [API Limitations](LIMITATIONS.md).
