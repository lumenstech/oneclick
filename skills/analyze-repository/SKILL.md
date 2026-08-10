---
name: analyze-repository
description: Analyze a local or GitHub repository with OneClick and explain its detected runtime, deployment path, ports, secret names, hardware signals, warnings, and evidence without exposing secret values.
---

Use `oneclick analyze <path-or-github-url> --format=json`.

Treat the generated evidence and warnings as the basis for conclusions. Do not claim that a repository is deployable unless `runtime.deployable` is true. Do not infer exact model VRAM requirements when OneClick only reports a GPU signal.
