# OneClick

**GitHub → deployment plan → hardware preflight → local deployment.**

OneClick is the open deployment engine behind DeployLocal. It analyzes a repository, identifies the runtime and infrastructure it appears to need, produces a normalized deployment plan, checks the target machine, and can execute supported Docker workloads on infrastructure the user controls.

## V0.1 scope

Supported for analysis:

- Dockerfile
- Docker Compose
- Node.js / pnpm / npm / yarn
- Python
- Go
- Rust
- Ollama-oriented repos
- vLLM/CUDA-oriented repos

Supported for execution:

- Docker Compose repositories
- Dockerfile repositories

Other detected runtimes generate a deployment plan but fail closed at execution until an executor is implemented.

## Design rules

- No GitHub Actions are required.
- No customer PAT is required by the CLI. Public GitHub repositories are shallow-cloned with `git`.
- Private repositories use the user's existing Git credential helper / SSH configuration; the CLI does not store GitHub credentials.
- No inbound SSH or public control port is opened by OneClick.
- Secrets are detected by **name only** from `.env.example`; secret values are never read into the plan.
- Deployment requires an explicit `--yes` confirmation flag.
- Docker/Compose commands are printed before execution.

## Build

```bash
go build -o oneclick ./cmd/oneclick
```

## Analyze

Local repository:

```bash
./oneclick analyze .
```

Public GitHub repository:

```bash
./oneclick analyze https://github.com/owner/repo
```

YAML-like human output:

```bash
./oneclick analyze . --format=yaml
```

Machine-readable JSON:

```bash
./oneclick analyze . --format=json
```

## Hardware preflight

```bash
./oneclick preflight .
```

The preflight reports host CPU, RAM, disk, architecture, Docker availability, and NVIDIA GPU/VRAM when `nvidia-smi` is available. V0.1 intentionally does not invent precise GPU requirements from source code; it reports GPU signals and warnings. CPU, RAM, and disk requirements are heuristic planning values derived from repository signals, not benchmarked sizing.

## Deploy

Docker Compose:

```bash
./oneclick deploy . --yes
```

Dockerfile:

```bash
./oneclick deploy . --yes --port=3000
```

For a Dockerfile with one detected `EXPOSE` port, `--port` changes the host-side port while preserving the detected container port.

`deploy` runs only when the repository has a supported Docker execution path. It does not execute arbitrary scripts from `package.json`, `Makefile`, or repository documentation.

## OneClick manifest

`analyze` emits the normalized fields described by [`schema/oneclick.schema.json`](schema/oneclick.schema.json). Repositories may later carry a hand-authored `oneclick.json`; V0.1 treats generated analysis as the source of truth and does not yet consume overrides.

## Agent Plugin

This repository is also an Agent Plugins 1.0.0 skills package. The portable plugin contains skills only; it deliberately carries no credentials. Agent Plugins v1 keeps authorization client-managed.

## DeployLocal relationship

OneClick is the open engine. DeployLocal is intended to provide the commercial control plane around it: GitHub App installation, node enrollment, remote deployment orchestration, managed monitoring, backup, hardware fit/quoting, and support.

See [`docs/architecture.md`](docs/architecture.md) and [`docs/security.md`](docs/security.md).
