---
name: prepare-local-deployment
description: Prepare a safe local deployment of a repository with OneClick after analysis and hardware preflight, using only a supported Dockerfile or Docker Compose executor.
---

1. Run analysis.
2. Run preflight.
3. Show the user the source, executor, ports, secret names, warnings, and plan hash.
4. Never ask OneClick to execute a repository whose `runtime.deployable` is false.
5. Deployment requires explicit user approval and the CLI `--yes` flag.
6. Never paste credentials into the command line or plugin package.
