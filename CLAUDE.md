# Claude Code Instructions

## Before Pushing Changes

Always run the following linters before pushing any changes:

```bash
make lint           # Go linting
make lint/actions   # GitHub Actions linting
```

## Project Overview

This is the Mondoo Operator - a Kubernetes operator for Mondoo security scanning.

## Common Commands

- `make test` - Run unit tests
- `make lint` - Run Go linter
- `make lint/actions` - Lint GitHub Actions workflows
- `make build` - Build the operator
- `make docker-build` - Build container image
