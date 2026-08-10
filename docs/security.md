# Security posture

## V0.1 invariants

- Analysis reads repository files but skips dependency/build directories.
- Source scanning is bounded to selected text-like files and file-size limits.
- `.env` is ignored. Only example environment files are parsed, and only variable names are retained.
- Git cloning disables terminal prompting so OneClick never solicits a password into its process.
- The execution engine supports only Docker Compose and Dockerfile paths.
- OneClick does not execute `package.json` scripts, Makefiles, shell scripts, README commands, or arbitrary detected install commands.
- `deploy` requires `--yes`.
- Commands are invoked with `exec.Command`; repository-derived strings are not concatenated into a shell command.
- No GitHub Actions are required.
- No inbound management port is opened.

## Trust boundary

Docker builds and Compose files are executable code supplied by the repository. A user who runs `oneclick deploy` is authorizing that repository's container build/runtime behavior on the target machine. Analysis and preflight are lower-risk operations; deployment is consequential and should be approval-gated in the managed DeployLocal product.

## Planned managed architecture

DeployLocal Node should:

1. authenticate outbound to the control plane;
2. receive an immutable deployment plan tied to a repository commit SHA;
3. verify plan identity/hash;
4. require policy/TrustAccept approval where configured;
5. execute locally;
6. health-check candidate state;
7. promote or roll back;
8. report sanitized status and logs.
