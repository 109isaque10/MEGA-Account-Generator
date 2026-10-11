<h1 align="center">
  <img src="./img/readme-icon.png" alt="MEGA Account Manager" width="256">
  <br>
  MEGA Account Manager & Keep-Alive (Go)
</h1>

<h4 align="center">High-performance Golang service for generating MEGA.nz accounts via Mailgun inbound webhooks and maintaining account activity with low-overhead periodic background logins.</h4>

<p align="center">
  <a href="#key-features">Key Features</a> •
  <a href="#environment-variables">Environment Variables</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#mailgun-webhook-setup">Mailgun Webhook</a> •
  <a href="#docker--scratch-build">Docker Scratch</a> •
  <a href="#github-actions">CI/CD</a> •
  <a href="#license">License</a>
</p>

---

## Key Features

* **High-Performance Go Daemon**: Self-contained Go executable with embedded dynamic Web UI console and REST API.
* **Mailgun Inbound Webhooks**: Instant, rate-limit-free verification using your custom domain rather than disposable email providers.
* **Predefined Email Patterns**: Custom pattern support via `EMAIL_PATTERN` (default `mega%d`) and `EMAIL_DOMAIN`.
* **Low-Resource Periodic Keep-Alive**: Background worker parks goroutines and wakes up every N days (default `30` days) to perform non-intrusive login checks, preventing MEGA inactivity account deletions.
* **Dual Storage & Backward Compatibility**: Synchronously maintains `accounts.csv` (matching original legacy schema), `accounts.json` with full metadata timestamps, and instant CSV/JSON exports.
* **Multi-Language Dynamic UI**: Dynamic translation architecture (English, Portuguese, Spanish) without fixed strings.
* **True Scratch Docker Image**: Multi-stage build producing an ultra-lightweight (~30MB) container `FROM scratch` with static Go binary and SSL root certificates.
* **GitHub Actions Workflow**: Automated multi-arch builder pushing directly to GitHub Container Registry (`ghcr.io`).

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `3000` | HTTP port for web UI and webhook receiver |
| `EMAIL_PATTERN` | `mega%d` | Format string for sequential addresses (e.g. `mega%d`, `cloud-%03d`) |
| `EMAIL_DOMAIN` | `example.com` | Domain configured in Mailgun for receiving emails |
| `SIGNIN_INTERVAL_DAYS` | `30` | Interval in days between periodic keep-alive login runs |
| `MAILGUN_API_KEY` | *(optional)* | Mailgun API key |
| `MAILGUN_WEBHOOK_SIGNING_KEY` | *(optional)* | Mailgun webhook signing key for HMAC-SHA256 signature verification |
| `MEGATOOLS_PATH` | `megatools` | Path to megatools binary |
| `MEGA_DEFAULT_PASSWORD` | *(optional)* | Password for generated accounts (random generated if blank) |
| `DATA_DIR` | `.` | Storage directory for `accounts.csv` and `accounts.json` |

---

## Quick Start

### Running with Go

```bash
# 1. Install megatools
# Ubuntu / Debian:
sudo apt-get install -y megatools

# 2. Configure environment
export EMAIL_DOMAIN="mycustomdomain.com"
export EMAIL_PATTERN="mega%d"
export SIGNIN_INTERVAL_DAYS=30

# 3. Build & Run
go build -o mega-manager ./cmd/server
./mega-manager
```

Visit `http://localhost:3000` in your browser.

---

## Mailgun Webhook Setup

1. In your **Mailgun Control Panel**, navigate to **Receiving** -> **Routes** (or Webhooks).
2. Create a route to match recipient emails matching your pattern (e.g. `match_recipient(".*@mycustomdomain.com")`).
3. Set the action to **Forward**:
   ```
   forward("https://your-domain.com/api/webhooks/mailgun")
   ```
4. When MEGA sends the verification email, Mailgun automatically posts the parsed message to your server, which extracts the confirmation URL and finalizes the account setup instantly!

---

## Docker Compose (Recommended for Homelab)

Launch instantly using the multi-arch `ghcr.io` image (`linux/amd64` and `linux/arm64`):

```bash
# 1. Edit compose.yml with your EMAIL_DOMAIN and optional Mailgun keys
# 2. Start the daemon
docker compose up -d
```

---

## Docker & Multi-Arch Scratch Build

Build the multi-architecture container locally (`linux/amd64` and `linux/arm64`):

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t mega-manager:latest .
```

---

## GitHub Actions CI/CD

The workflow located at `.github/workflows/docker.yml` automatically builds multi-arch images (`linux/amd64,linux/arm64`) on push to `main`/`master` or release tags, publishing them directly to GitHub Container Registry (`ghcr.io`).

---

## License

[MIT License](LICENSE)
