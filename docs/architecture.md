# Architecture

## Product boundary

OneClick is an open deployment engine. DeployLocal is the commercial orchestration and services layer.

```text
GitHub/local repo
      |
      v
 OneClick analyzer
      |
      +--> normalized plan + evidence
      |
      v
 hardware preflight
      |
      v
 supported executor
      |
      v
 customer-controlled Docker host
```

DeployLocal can later wrap OneClick with:

- GitHub App repository access using installation tokens
- enrolled DeployLocal nodes using outbound-only control connections
- inventory and hardware fit analysis
- deployment history and signed plan hashes
- monitoring and backup
- managed updates and rollback
- quote generation when target hardware is insufficient
- TrustAccept approval gates for consequential actions

## V0.1 executors

`docker-compose` executes `docker compose up -d --build`.

`docker` builds an OCI image from the repository Dockerfile and starts one container with `--restart unless-stopped`. V0.1 maps a port only when exactly one exposed/detected port exists or the operator supplies `--port`.

Node/Python/Go/Rust repositories without Docker packaging are analyzed but are not executed. This is deliberate: OneClick does not run arbitrary repository scripts as a deployment mechanism.

## Future control plane

The intended DeployLocal control plane does not need inbound SSH. A node agent should establish an outbound authenticated connection, pull the exact approved commit, execute the OneClick plan locally, report health, and retain the previous deployment for rollback.
