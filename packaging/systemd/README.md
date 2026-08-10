# Linux service installation

V0.1 is a Linux + Docker host service. It is intentionally outbound-only.

## Service account

Create a dedicated account and grant it Docker access:

```bash
sudo useradd --system --home-dir /var/lib/oneclick-node --create-home --shell /usr/sbin/nologin oneclick-node
sudo usermod -aG docker oneclick-node
```

Membership in the `docker` group is **root-equivalent host authority** because the Docker daemon can create privileged containers and mount host paths. The dedicated Unix account reduces unrelated process/file access, but Docker access remains the real execution trust boundary. OneClick's Compose/Docker safety gates are therefore mandatory, not optional hardening.

Install both binaries and the unit:

```bash
sudo install -m 0755 oneclick /usr/local/bin/oneclick
sudo install -m 0755 oneclick-node /usr/local/bin/oneclick-node
sudo install -m 0644 packaging/systemd/oneclick-node.service /etc/systemd/system/oneclick-node.service
```

Create `/etc/oneclick-node.env` as root, mode `0600`:

```text
ONECLICK_CONTROL_URL=https://control.deploylocal.com
ONECLICK_ENROLLMENT_TOKEN=<one-time-token>
```

The enrollment token is needed only for first enrollment. Remove it from the environment file after `state.json` has been created successfully.

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now oneclick-node
sudo systemctl status oneclick-node
```

The node writes identity, command journal, cached results, and transient source workspaces below `/var/lib/oneclick-node` through systemd `StateDirectory=`.
