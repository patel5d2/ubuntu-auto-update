# Agent (Rust)

Memory-safe daemon that runs on each managed Ubuntu host. It enrolls with
the backend on first start, then on a systemd timer runs `apt update` /
`apt upgrade --dry-run` and POSTs the output back to `/api/v1/report`.

## Layout

```
src/
  main.rs            CLI entry point and command dispatch
  config.rs          TOML/env config loading
  enrollment.rs      One-shot POST to /api/v1/enroll, persists bearer token
  http_client.rs     reqwest wrapper with rustls + bearer auth
  updater.rs         Shells out to apt; collects stdout/stderr
  logging.rs         tracing-subscriber setup (json or text)
  metrics.rs         Prometheus counters
systemd/
  ubuntu-auto-update-agent.service
  ubuntu-auto-update-agent.timer
```

## Running locally

```bash
cd agent
cargo run -- --help                            # see all subcommands
cargo run -- generate-config --output agent.toml
cargo run -- enroll --backend http://localhost:8080 --token dev-enrollment-token
cargo run -- run                               # one-shot update cycle
```

## Tests

```bash
cargo test
cargo clippy --all-targets
```

## Installation on a real host

```bash
cargo build --release
sudo install -m 0755 target/release/ua-agent /usr/local/bin/
sudo install -m 0644 systemd/ubuntu-auto-update-agent.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ubuntu-auto-update-agent.timer
```

The agent expects its config at `/etc/ubuntu-auto-update/agent.toml` and
its enrollment token at `/var/lib/ubuntu-auto-update/auth.token`.
