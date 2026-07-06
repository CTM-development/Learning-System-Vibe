# Learning System

Personal learning system: markdown notes as source of truth, Anki-style spaced
repetition (FSRS), universal activity timing, PDF sources — served on the home
network as a single Go binary with an embedded React frontend.

Full spec and roadmap: [docs/plan.md](docs/plan.md).

## Build & run

Requires Go ≥ 1.26 and Node ≥ 20.

```sh
make build        # builds web/ (Vite) then embeds it into bin/learning-server
./bin/learning-server
# open http://<lan-ip>:8844
```

Development: run `go run ./cmd/server` and `cd web && npm run dev` side by
side; the Vite dev server proxies `/api` to :8844.

Tests: `make test`.

## Configuration

Optional YAML file passed via `-config path/to/config.yaml`; environment
variables override the file. Defaults in parentheses.

| YAML key          | Env var                | Default       |
|-------------------|------------------------|---------------|
| `port`            | `LEARN_PORT`           | `8844`        |
| `notes_dir`       | `LEARN_NOTES_DIR`      | `notes`       |
| `attachments_dir` | `LEARN_ATTACHMENTS_DIR`| `attachments` |
| `db_path`         | `LEARN_DB_PATH`        | `learning.db` |
| `new_per_day`     | `LEARN_NEW_PER_DAY`    | `10`          |

Everything on disk is portable: the notes directory (markdown), the
attachments directory (PDFs), and one SQLite file. Back up = copy those three.

## Run as a service (systemd)

`/etc/systemd/system/learning.service`:

```ini
[Unit]
Description=Personal learning system
After=network.target

[Service]
User=onestone
WorkingDirectory=/home/onestone/learning
ExecStart=/home/onestone/learning/learning-server -config /home/onestone/learning/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now learning
```

**Security note:** there is no login — the server is meant for a trusted home
LAN only. Never port-forward it. For remote access use Tailscale or similar.
