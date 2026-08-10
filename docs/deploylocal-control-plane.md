# DeployLocal control-plane boundary

OneClick is intentionally not the SaaS control plane.

The DeployLocal layer should own:

- accounts, organizations, roles, and billing/quotes;
- GitHub App installations and repository selections;
- node enrollment and device identity;
- hardware inventory and compatibility results;
- deployment records tied to commit SHA + OneClick plan hash;
- approval policy and TrustAccept integration;
- monitoring, backup, managed updates, support, and later rollback;
- hardware recommendation / quote handoff when preflight fails.

## Node communication

A DeployLocal Node should make an outbound authenticated TLS connection to the control plane. The design must not require opening SSH or a proprietary inbound management port on the customer's network.

## Suggested initial API resources

```text
POST /api/github/installations/:id/analyze
GET  /api/nodes
POST /api/nodes/:id/preflight
POST /api/deployments
GET  /api/deployments/:id
POST /api/deployments/:id/approve
POST /api/deployments/:id/execute
POST /api/quotes/hardware
```

`execute` should accept a deployment ID, not arbitrary repository commands. The server resolves the immutable source revision and approved plan from stored state.

## Quote-first conversion

When preflight does not fit, return structured deficits rather than a generic failure. DeployLocal can map those deficits into recommended hardware classes and create a quote request. OneClick itself should remain vendor-neutral and must not hard-code commercial SKUs.
