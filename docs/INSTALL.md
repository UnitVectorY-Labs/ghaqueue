---
layout: default
title: Installation
nav_order: 2
permalink: /install
---

# Installation
{: .no_toc }

## Table of Contents
{: .no_toc .text-delta }

- TOC
{:toc}

## Prerequisites

- **Go 1.27 or newer:** required when building from source
- **GitHub access:** a token that can read the repositories and Actions data being observed
- **git:** optional, used to infer the current repository from its `origin` remote
- **GitHub CLI (`gh`):** optional, used as an authentication fallback

## Install Using Go

```bash
go install github.com/UnitVectorY-Labs/ghaqueue@latest
```

## Build from Source

```bash
git clone https://github.com/UnitVectorY-Labs/ghaqueue.git
cd ghaqueue
go build -o ghaqueue .
./ghaqueue --version
```

Release binaries, when available, are published on [GitHub Releases](https://github.com/UnitVectorY-Labs/ghaqueue/releases).

## Authentication

Authentication uses the first nonempty value from:

1. `GHAQUEUE_TOKEN`
2. `GITHUB_TOKEN`
3. `GH_TOKEN`
4. `gh auth token --hostname github.com`

Without a token, public endpoints are attempted with GitHub's unauthenticated rate limits. Tokens are sent only to the fixed `api.github.com` endpoint. GitHub Enterprise hosts are not supported in v1.

Fine-grained tokens need repository **Actions: read** and access to each relevant repository. Organization runner inventory additionally requires **Self-hosted runners: read** at organization scope; repository runner inventory requires repository **Administration: read**. GitHub also applies the caller's access requirements. Classic tokens generally need `repo` for private repository data and `admin:org` for organization runners. Organization policies and SSO authorization may impose additional restrictions.

Runner permissions are optional for other views. Only the `runners` command requests inventory. See [GitHub's runner endpoint permissions](https://docs.github.com/en/rest/actions/self-hosted-runners) and [workflow endpoint permissions](https://docs.github.com/en/rest/actions/workflow-runs).
