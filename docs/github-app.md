# GitHub App integration contract

The DeployLocal commercial control plane should use a GitHub App. OneClick itself does not need a customer PAT.

## Minimum repository permissions

- Metadata: read
- Contents: read

Optional later permissions:

- Checks: write, only if DeployLocal posts deployment status back to GitHub

## Events

Subscribe only when the customer enables automatic deployment:

- `push`
- `release`

The webhook should enqueue a deployment candidate tied to the exact repository and commit SHA. It must not execute immediately when the customer's policy requires approval.

## Flow

1. Customer installs the DeployLocal GitHub App.
2. Customer selects repositories explicitly.
3. DeployLocal obtains a short-lived installation token server-side.
4. Control plane reads repository metadata/content or gives the enrolled node an ephemeral source-fetch grant.
5. OneClick analyzes the exact commit.
6. DeployLocal stores the normalized plan and its hash.
7. Hardware preflight runs against the selected node.
8. Policy/TrustAccept approval runs where configured.
9. The node executes the immutable approved revision locally.

Installation tokens, OAuth tokens, deploy keys, and customer secrets must never appear in a OneClick manifest or Agent Plugin package.
