# git-synced

A CLI tool that reports on merge requests created by a set of authors across one or more GitLab repositories within a configurable lookback window.

> **Note:** Only GitLab is supported currently.

## Setup

Copy the example config and fill in your details:

```bash
cp config.example.yaml config.yaml
```

```yaml
format: "text"         # text | json | yaml
providers:
  - name: gitlab
    token: "glpat-xxxxxxxxxxxx"
    base_url: "https://gitlab.com"
    lookback_hours: 96
    state: "all"       # opened | closed | merged | all
    authors:
      - alice
      - bob
    repos:
      - group/repo
      - group/*        # wildcard expands all projects under a group
```

## Usage

```bash
go run ./cmd --config config.yaml
```

Or build first:

```bash
go build -o git-synced ./cmd
./git-synced --config config.yaml
```

## Cron

To run automatically each morning:

```
0 9 * * * cd /path/to/git-synced && ./git-synced >> /tmp/mr-report.log 2>&1
```
