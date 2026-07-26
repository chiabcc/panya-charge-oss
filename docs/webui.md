# Config WebUI

The embedded WebUI lets you edit `config.yaml` from a browser and applies changes with minimal disruption — hot-reloading what can be hot-reloaded, and warning before changes that require a charger reconnect or process restart.

Disabled by default. No database, no accounts — just a token.

![WebUI config page](images/webui-config.png)

## Enable

Add to `config.yaml`:

```yaml
webui:
  enabled: true
  listen: "127.0.0.1:8888"
  token: "a-strong-secret-token"
```

Then access at `http://127.0.0.1:8888`.

### LAN access

To bind on a non-loopback address (e.g., `0.0.0.0:8888` for access from other machines on your network), a token is **required** — the server will refuse to start without one. This prevents running an unauthenticated config endpoint on your LAN.

```yaml
webui:
  enabled: true
  listen: "0.0.0.0:8888"
  token: "a-long-random-string-here"
```

### Environment overrides

All WebUI settings can be set via environment variables instead of YAML:

```bash
PANYA_WEBUI_ENABLED=true
PANYA_WEBUI_LISTEN=127.0.0.1:8888
PANYA_WEBUI_TOKEN=your-secret
```

When a config field is overridden by an environment variable, it appears **disabled** in the UI with an `ENV` badge — you must change the environment variable, not the form.

## Login

![WebUI login page](images/webui-login.png)

Enter the token you configured. The session lasts 24 hours via an `HttpOnly` + `SameSite=Strict` cookie. On loopback without a token set, the config page is served directly (no login).

## How changes apply

Every field shows a badge indicating what happens when you change it:

| Badge | Meaning | Examples |
|---|---|---|
| **instant** | Applied immediately without charger disruption | `min_amps`, `max_amps`, `log_level` |
| **disconnect** | Charger briefly disconnects (~5s) while the bridge rebuilds | `mqtt.broker`, `ocpp_port`, `base_topic` |
| **restart** | Saved to disk — takes effect on next process restart | `webui.enabled`, `webui.listen` |

When a change requires a rebuild and a charging session is active, the UI shows a confirmation prompt listing the affected fields before proceeding.

### Secrets

`mqtt.password` and `webui.token` are **write-only** — they never appear in HTTP responses. The form shows a password field with a placeholder; leave it blank to keep the current value.

## Architecture

- **Go `html/template`** + vendored [htmx](https://htmx.org) for partial swaps
- **stdlib `net/http`** only — no framework, no frontend build step
- Templates and static assets are embedded via `embed.FS` (single binary)
- Content negotiation: `Accept: text/html` renders the form, `Accept: application/json` returns the config as JSON for programmatic access

### API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/config` | Effective config (HTML form or JSON via Accept header) |
| `POST` | `/api/config` | Save config (htmx partial or JSON response) |
| `POST` | `/login` | Authenticate with token |
| `POST` | `/logout` | Clear session |

```bash
# Read config as JSON
curl -H "Accept: application/json" -b "panya_webui_session=<token>" \
  http://127.0.0.1:8888/api/config
```

## Design

The interface follows a **Graphite & Amber** design system rooted in the charger's physical world — brushed-aluminium surfaces, a single amber accent (the charging LED color), and elevation-based depth instead of decorative borders. See [`.interface-design/system.md`](../.interface-design/system.md) for the full token system.
