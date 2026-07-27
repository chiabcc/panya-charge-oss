# Testing the HA Add-on

How to test the panya-charge-oss Home Assistant add-on before merging to main.

## Quick Reference

| Method | What it tests | Setup effort | Inner loop |
|---|---|---|---|
| [Docker smoke test](#docker-smoke-test) | Binary + status page in HA image | None | Seconds |
| [Samba share](#samba-share-recommended) | Full add-on in HAOS VM | Moderate | Seconds |
| [Tag-triggered CI](#tag-triggered-ci) | Multi-arch image via GHCR | Low | Minutes |

---

## Docker Smoke Test

Validates the Dockerfile build and status page without HA Supervisor. Fastest way to check the binary works inside the HA base image.

### Prerequisites

- Docker running locally
- MQTT broker reachable (Mosquitto, HA, or the dev compose stack)

### Steps

1. Build the add-on image:

   ```bash
   docker build -f ha-addon/Dockerfile -t panya-addon:local .
   ```

2. Run with env vars (bypasses bashio, which requires Supervisor):

   ```bash
   docker run --rm -p 8887:8887 -p 8888:8888 \
     --add-host=host.docker.internal:host-gateway \
     --entrypoint /usr/local/bin/panya-charge-oss \
     -e PANYA_WEBUI_ENABLED=false \
     -e PANYA_WEBUI_STATUS_ENABLED=true \
     -e PANYA_WEBUI_LISTEN=0.0.0.0:8888 \
     -e PANYA_MQTT_BROKER=tcp://host.docker.internal:1883 \
     panya-addon:local -config ""
   ```

3. Open the status page:

   ```
   http://localhost:8888/status
   ```

4. Point your charger at:

   ```
   ws://<your-host-ip>:8887/{ws}
   ```

### What this does NOT test

- bashio launcher (`run.sh`)
- HA schema form configuration
- HA ingress proxy (`X-Ingress-Path` header)
- MQTT Services API auto-discovery

---

## Samba Share (Recommended)

Mounts your local `ha-addon/` folder into a HAOS VM so the add-on appears in HA's Add-on Store without pushing to GitHub. Best inner loop for iterative development.

### Prerequisites

- HAOS VM running (VirtualBox, Proxmox, or KVM)
- HA onboarding completed (account created)
- MQTT integration configured in HA

### One-Time Setup

1. In HAOS, install the Samba share add-on:

   - Settings → Add-ons → Add-on Store → search "Samba share" → Install → Start
   - Note the username (your HA account) and password

2. On your machine, mount HA's addons folder:

   ```bash
   mkdir -p /mnt/ha-addons

   # Linux
   sudo mount -t cifs //homeassistant.local/addons /mnt/ha-addons \
     -o username=<ha-user>,password=<ha-password>,uid=$(id -u)

   # macOS
   mkdir -p /Volumes/ha-addons
   mount_smbfs //<ha-user>:<ha-password>@homeassistant.local/addons /Volumes/ha-addons
   ```

3. Symlink your repo's add-on folder:

   ```bash
   # Linux
   ln -s /home/takumi/workspaces/panya-charge-oss/ha-addon \
     /mnt/ha-addons/panya-charge-oss

   # macOS
   ln -s /Users/takumi/workspaces/panya-charge-oss/ha-addon \
     /Volumes/ha-addons/panya-charge-oss
   ```

4. In HAOS, refresh the Add-on Store:

   - Settings → Add-ons → Add-on Store → ⋮ → **Check for updates**
   - "Panya Charge OSS" appears under **Local add-ons**

5. Install, configure MQTT + charging params in the schema form, start.

### Iterating

After code changes, rebuild and restart:

```bash
docker build -f ha-addon/Dockerfile -t panya-addon:dev .
```

Then in HA: click **Restart** on the add-on page. Changes take effect immediately.

### Testing Ingress

Once the add-on is running:

1. Click **Open Web UI** on the add-on page
2. The status page loads via HA ingress at `/api/hassio_ingress/<session>/status`
3. Verify: OCPP URL, MQTT badge, chargers table, smart charging readings

### Unmount

```bash
# Linux
sudo umount /mnt/ha-addons

# macOS
umount /Volumes/ha-addons
```

---

## Tag-Triggered CI

Pushes a git tag to trigger the GitHub Actions multi-arch build. The image lands in GHCR, then HAOS installs it from the repo URL. Slower but tests the full CI pipeline.

### Prerequisites

- Branch pushed to GitHub (does not need to be merged to main)
- GitHub Actions workflow exists at `.github/workflows/ha-addon-build.yml`

### Steps

1. Push a test tag:

   ```bash
   git tag v0.0.1-test
   git push origin v0.0.1-test
   ```

2. Wait for CI to complete:

   ```bash
   gh run watch
   ```

   Or check: https://github.com/chiabcc/panya-charge-oss/actions

3. In HAOS, add the repository (if not already added):

   - Settings → Add-ons → Add-on Store → ⋮ → Repositories
   - Add: `https://github.com/chiabcc/panya-charge-oss`

4. Find "Panya Charge OSS" → Install → Configure → Start

5. The add-on pulls the multi-arch image from GHCR automatically.

### Cleanup

Delete the test tag after verification:

```bash
git tag -d v0.0.1-test
git push origin :refs/tags/v0.0.1-test
```

---

## What to Verify

Regardless of method, check these:

| Check | How |
|---|---|
| Add-on starts without errors | Add-on logs show `mqtt connected`, `ocpp csms server started` |
| Schema form works | Change a value, click Save, add-on restarts |
| Status page loads | Click "Open Web UI" or visit `/status` directly |
| Charger connects | Logs show `charger connected`, `boot notification` |
| MQTT auto-discovery works | HA entities appear (Settings → Devices → Panya) |
| OCPP WebSocket reachable | Charger URL `ws://<HA-IP>:8887/{ws}` accepts connections |
| Config changes propagate | Change `min_amps` in HA form → verify in status page after restart |

## Common Issues

| Symptom | Cause | Fix |
|---|---|---|
| `Could not resolve host: supervisor` | Running HA image outside Supervisor | Use `--entrypoint` to bypass bashio (smoke test) or run in HAOS |
| Status page shows "No chargers connected" | Charger WebSocket connected but BootNotification not yet received | Wait a few seconds, refresh. Should show after `boot notification` log line |
| Add-on won't install | MQTT not configured in HA | Install Mosquitto add-on or configure HA MQTT integration first |
| `ingress: false` in manifest | Stale manifest from older version | Rebuild image, ensure `ha-addon/config.yaml` has `ingress: true` |
| Port 8887 unreachable from charger | HAOS firewall or bridge networking | Use HA IP, not add-on DNS name. Check `host_network: false` is expected |
