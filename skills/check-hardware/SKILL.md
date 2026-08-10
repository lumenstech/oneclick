---
name: check-hardware
description: Compare a repository's OneClick deployment plan with the current machine and report whether CPU, memory, disk, Docker, and detected NVIDIA GPU requirements are satisfied.
---

Run `oneclick preflight <path-or-github-url>` and explain `fits`, `problems`, and `warnings` exactly. Do not convert an unknown memory or GPU value into a pass claim.
