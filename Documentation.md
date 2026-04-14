# Grocery List

A locally-run, mobile-friendly grocery list app with group-based organization,
drag-to-reorder, and a Go backend with optional TLS. No authentication required.

---

## Quick Start (HTTP, for testing)

```bash
cd /opt/grocery-list
go mod tidy
make run
# Open http://localhost:8080
```

---

## Config File

Generate a default config at `~/.grocery.json`:

```bash
make init-config
```

Edit it to taste:

```json
{
  "port": 8443,
  "tls_cert": "~/certs/cert.pem",
  "tls_key":  "~/certs/key.pem",
  "static_dir": "/opt/grocery-list/web",
  "data_file":  "/opt/grocery-list/items.json",
  "groups": [
    "Produce",
    "Meats",
    "mid store",
    "back wall",
    "frozen",
    "deli area near front"
  ]
}
```

- `port` — listen port (default 8080)
- `tls_cert` / `tls_key` — paths to your PEM cert/key; if both present, HTTPS is used automatically
- `static_dir` — where `index.html`, `style.css`, `app.js` live
- `data_file` — JSON flat file for persistence; created automatically on first write
- `groups` — ordered list of grocery store sections shown in the UI

---

## Running with TLS

Place your PEM cert and key somewhere readable by the service user, then:

```bash
make run-tls
# Uses ~/.grocery.json — make sure tls_cert and tls_key are set
```

Or override on the command line:

```bash
./bin/grocery-list --tls-cert ~/certs/cert.pem --tls-key ~/certs/key.pem --port 8443
```

---

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `~/.grocery.json` | Config file path |
| `--port` | *(from config)* | Override listen port |
| `--tls-cert` | *(from config)* | Override TLS cert path |
| `--tls-key` | *(from config)* | Override TLS key path |
| `--web-dir` | *(from config)* | Override static assets directory |
| `--data-file` | *(from config)* | Override JSON data file path |
| `--init-config` | — | Write default `~/.grocery.json` and exit |

CLI flags always win over config file values.

---

## systemd Install (Linux)

```bash
# Create a dedicated service user
sudo useradd -r -s /usr/sbin/nologin grocery

# Write a config for the service user
sudo -u grocery bash -c 'cat > /home/grocery/.grocery.json' << EOF
{
  "port": 8443,
  "tls_cert": "/home/grocery/certs/cert.pem",
  "tls_key":  "/home/grocery/certs/key.pem",
  "static_dir": "/opt/grocery-list/web",
  "data_file":  "/opt/grocery-list/items.json",
  "groups": ["Produce","Meats","mid store","back wall","frozen","deli area near front"]
}
EOF

# Install binary, web assets, and unit file
make install

# Enable and start
sudo systemctl enable --now grocery-list

# Watch logs
sudo journalctl -u grocery-list -f
```

---

## REST API

| Method | Path | Body | Description |
|--------|------|------|-------------|
| GET | `/api/config` | — | Returns `{groups: [...]}` |
| GET | `/api/items` | — | All items as JSON array |
| POST | `/api/items` | `{name, group}` | Add item; returns saved item |
| PATCH | `/api/items/:id` | `{state?, completed?}` | Update state/completed |
| DELETE | `/api/items/:id` | — | Delete item (204) |
| POST | `/api/move` | `{id, group, order_ids}` | Move item to another group |
| POST | `/api/reorder` | `{group, ids}` | Reorder items within a group |
| POST | `/api/sync` | `[...items]` | Bulk merge client state → server |

---

## UI Usage

- **Tap item name or badge** — cycles state: `Needed` (green) → `Check` (yellow) → `Not Needed` (red)
- **Tap checkbox** — marks item complete (strikethrough)
- **Tap trash icon** — deletes item
- **Drag handle** (∷ dots) — reorder within a group, or drag to another group's header to move it
- **Sync toggle** (top right) — when OFF, changes are local only; toggling back ON re-fetches from server
- **Group headers** — tap to collapse/expand
