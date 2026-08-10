# Security posture

## V0.1 invariants

- Analysis reads repository files but skips dependency/build directories.
- Source scanning is bounded to selected text-like files and file-size limits.
- `.env` is ignored. Only example environment files are parsed, and only variable names are retained.
- Git cloning disables terminal prompting so OneClick never solicits a password into its process.
- The execution engine supports only Docker Compose and Dockerfile paths.
- OneClick does not execute `package.json` scripts, Makefiles, shell scripts, README commands, or arbitrary detected install commands.
- `deploy` requires `--yes`.
- Local Git deployment refuses a dirty working tree; commit or stash changes first. Analysis may inspect a dirty tree, but marks it explicitly and warns that `HEAD` does not bind those modifications.
- Managed execution can supply a canonical source reference, exact source revision, and `--expected-plan-hash`; preflight/deploy refuse a plan-hash mismatch before Docker runs.
- The CLI emits an authorization-grade `StablePlanHash` that removes checkout-directory variance from fallback service names while preserving explicit service/runtime identities.
- Commands are invoked with `exec.Command`; repository-derived strings are not concatenated into a shell command.
- Docker/Compose child commands run with an isolated HOME and a minimal environment; the repository does not inherit arbitrary OneClick/DeployLocal process secrets.
- No GitHub Actions are required.
- No inbound management port is opened.

## Compose trust gate

Docker Compose treats a Compose file as executable/trusted input. OneClick therefore does not pass an arbitrary repository Compose file directly to `docker compose up`.

V0.1 first performs a raw-file gate that rejects features which can read host files, import transitive configuration, expose the engine socket, or execute host-side provider helpers. This includes `include`, `extends`, `env_file`, `label_file`, file-backed `secrets`/`configs`, `provider`, `credential_spec`, `use_api_socket`, Docker socket paths, and environment interpolation.

For files that pass the raw gate, OneClick renders one explicitly selected Compose file with an isolated project name using:

`docker compose -p <oneclick-project> -f <compose-file> config --format json --no-env-resolution --no-interpolate --no-path-resolution`

The canonical model is then rejected if it requests host-level/elevated surfaces such as privileged mode, added capabilities, security options, host namespaces/network modes, direct host devices, bind mounts, external volumes/networks, unsafe resource names, volume/network driver options, provider/file-reference surfaces, build entitlements/SSH/additional contexts, host build networking, or build paths escaping the repository. NVIDIA GPU reservations through the Compose deploy reservation model remain allowed; arbitrary direct device mappings do not.

Only after both checks succeed does OneClick execute `docker compose ... up -d --build`. The explicit `-f` prevents an unreviewed default override file from being merged automatically.

## Dockerfile executor

The Dockerfile executor does not add host mounts or privileged mode. Containers are started with `no-new-privileges` in addition to Docker's normal isolation. V0.1 does not claim that arbitrary container images are risk-free; kernel/container-runtime isolation remains part of the target host's security boundary.

## Trust boundary

A user who runs `oneclick deploy` is still authorizing repository-supplied build/application code to execute inside containers on the target machine. The V0.1 gates prevent known host-escape-by-configuration classes from being granted intentionally through OneClick, but they are not a substitute for container-runtime patching, image trust, or application security review.

Analysis and preflight are lower-risk operations. Deployment is consequential and should be approval-gated in the managed DeployLocal product.

## Managed architecture

DeployLocal Node should:

1. authenticate outbound to the control plane;
2. receive a signed immutable deployment command tied to a repository, exact commit SHA, and plan hash;
3. fetch only that exact revision with a short-lived repository credential;
4. independently analyze the fetched source and enforce the approved plan hash;
5. require policy/TrustAccept approval where configured;
6. execute locally through OneClick;
7. journal command identity before execution and refuse replay;
8. health-check candidate state when a supported health contract exists;
9. report sanitized status without forwarding repository build logs by default.

Rollback is not implemented in V0.1 and must not be represented as available until a tested rollback executor exists.
