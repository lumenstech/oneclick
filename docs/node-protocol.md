# OneClick Node protocol v0.1

The node is an outbound-only Linux deployment executor. It exposes no HTTP listener and does not require inbound SSH from DeployLocal.

## Identity

On first enrollment the node generates an Ed25519 keypair locally. The private key is written `0600` and never sent to the control plane. The one-time enrollment token is sent only over HTTPS. The enrollment response pins the control-plane Ed25519 public key.

After enrollment every node->control request is signed over:

`METHOD + "\n" + request-uri + "\n" + timestamp + "\n" + nonce + "\n" + sha256(body)`

with headers `X-OneClick-Node-Id`, `X-OneClick-Timestamp`, `X-OneClick-Nonce`, `X-OneClick-Body-SHA256`, and `X-OneClick-Signature`.

## Commands

The control plane returns `{payload, signature}`. `payload` is base64url of the exact JSON bytes; `signature` is an Ed25519 signature over those decoded bytes. The node verifies the pinned key before parsing or executing. Commands expire after at most 15 minutes and are journaled durably **before execution** to refuse replay after restarts.

V0.1 supports only `deploy` commands with:

- `repository`: canonical GitHub `owner/repo`
- exact 40- or 64-hex `commit_sha`
- 64-hex approved `plan_hash`
- optional host-port override

The node passes `https://github.com/<owner>/<repo>`, the exact revision, and the approved hash back into local OneClick preflight/deploy. OneClick independently re-analyzes the fetched source with that canonical identity and refuses execution when the stable plan hash differs. A signed `plan_hash` is therefore an execution condition, not just audit metadata.

## GitHub source credentials

The node requests a source credential only after accepting a signed command. The control plane should mint a GitHub App installation token narrowed to the selected repository with read-only Contents permission.

The installation token is treated as opaque and held only in memory. It is sent as an `Authorization` header to GitHub's repository-archive API for the exact commit SHA. The GitHub API returns a temporary archive redirect; the node follows only the expected archive host and deliberately does **not** forward the installation token to that redirected request.

The token is never written into a Git URL, process command line, log, plan, or persistent file.

The archive extractor rejects path traversal, symlinks/hardlinks, oversized archives, excessive file counts, Git submodules, and Git LFS pointers in v0.1.

## Exactly-once execution behavior

Each command ID is reserved in an append-only `0600` journal before source retrieval or execution. A second delivery of the same ID never reruns OneClick. If a prior result was stored but its upload failed, the cached result is resent instead.

This is command-level replay protection, not a claim that arbitrary application side effects are transactionally reversible. Rollback is not implemented in V0.1.

## Execution and output handling

The node runs local OneClick preflight, then OneClick deploy with the canonical source identity and expected plan hash. The OneClick child receives a minimal environment rather than the node/control-plane environment. Child stdout/stderr are not sent to the control plane in V0.1 because repository build output may contain secrets.

OneClick itself performs the Compose trust gate before Docker execution and runs Docker/Compose with an isolated environment. Unsafe Compose host-access features fail closed.

## Control-plane endpoints

- `POST /v1/nodes/enroll`
- `POST /v1/nodes/{node_id}/heartbeat`
- `GET /v1/nodes/{node_id}/commands?wait_seconds=25`
- `GET /v1/nodes/{node_id}/commands/{command_id}/source-credential`
- `POST /v1/nodes/{node_id}/commands/{command_id}/result`

The commercial DeployLocal control plane owns enrollment-token issuance, GitHub App installation handling, short-lived repository credentials, command signing, approvals, job orchestration, monitoring, quotes, and support. Those commercial services do not belong in the public OneClick repository.

## Host privilege boundary

The systemd unit runs as a dedicated `oneclick-node` Unix user rather than root, with additional service hardening. It requires membership in the `docker` group to execute deployments. Docker-group access is root-equivalent host authority; the Compose/Docker execution policy is therefore part of the security boundary, not merely a convenience check.
