# MEGA Account Generator & Keep-Alive Daemon in Go

A lightweight, concurrent Go service and web dashboard that automates MEGA.nz account generation via Mailgun inbound webhooks, maintains account activity with low-overhead periodic background logins, persists credentials in backward-compatible CSV, SQLite, and JSON formats, and packages into a minimal `scratch` Docker image via GitHub Actions.

## User Review & Critical Decisions

> [!IMPORTANT]
> The following decisions were clarified and confirmed in Phase 1:

- **Operation Mode**: Embedded web dashboard with REST API, real-time live activity logs, generator controls, and account management.
- **Keep-Alive Interval**: Configurable via environment variable (`SIGNIN_INTERVAL_DAYS`, default `30` days) with cron/ticker running in a low-resource sleep state between runs.
- **Storage Strategy**: Dual-storage engine writing to `accounts.csv` (matching original schema: `Email, MEGA Password, Usage, Mail.tm Password, Mail.tm ID, Purpose`), an SQLite database (`accounts.db`) with full search and metadata, and instant JSON export endpoint (`/api/accounts/export.json`).
- **Email Generation Pattern**: Predefined pattern configurable via `EMAIL_PATTERN` (default: `mega%d`) and `EMAIL_DOMAIN` (e.g. `example.com`), producing sequential addresses like `mega1@example.com`, `mega2@example.com`, or timestamped variations.
- **Inbound Verification**: Mailgun HTTP webhook endpoint (`/api/webhooks/mailgun`) with HMAC signature verification (`MAILGUN_API_KEY`) and fallback parser for incoming confirmation emails.
- **Docker & CI/CD**: Multi-stage Docker build producing a true `scratch` container with static Go binary, bundled static `megatools` binary, and `ca-certificates.crt`, plus `.github/workflows/docker.yml` for automated GitHub Actions builds.
- **I18n / Dynamic Strings**: All UI labels, logs, and notifications load dynamically through language dictionary bundles (English, Portuguese, Spanish) without fixed hardcoded strings, respecting the project guidelines.

---

## 1. Overview & Core Concept

- **What It Does**: Replaces the Python CLI scripts (`generate_accounts.py`, `signin_accounts.py`, `convert_csv.py`) with a single, high-performance Go binary. The binary exposes a modern web interface on port 3000, receives incoming MEGA confirmation emails via Mailgun webhook, coordinates account registration and verification pipelines, and sleeps in an ultra-low-memory idle state while running a background ticker to log into registered accounts every 30 days to prevent inactivity deletions.
- **Target Audience / Persona**: Developers, homelab operators, and power users who need automated MEGA storage account pools without manual email clicking or Python dependency overhead.
- **Key Value**: 
  - Zero Python or pip dependencies; single self-contained executable.
  - No dependency on third-party disposable email providers (e.g. Mail.tm rate limits or blocking); utilizes the user's custom domain via Mailgun.
  - Automated hands-off verification via webhooks.
  - Minimal resource footprint (~15-25MB RAM in idle state).
  - Deployable on bare metal or minimal scratch container.

---

## 2. User Experience & Visual Design

### Key User Flows
1. **Dashboard & Status View**: User visits `http://localhost:3000` and sees system health, total accounts, accounts pending verification, next scheduled keep-alive sign-in run, and current environment configuration (`EMAIL_DOMAIN`, `EMAIL_PATTERN`, `SIGNIN_INTERVAL_DAYS`).
2. **Account Generation Flow**: User triggers generation of N accounts (or custom password / thread concurrency) from the UI or via REST API (`POST /api/accounts/generate`).
   - The Go daemon generates the next email(s) according to pattern (`mega%d@domain.com`).
   - The daemon initiates registration using `megatools reg --scripted --register` and stores pending registration tokens in memory.
   - When Mailgun delivers the verification webhook to `/api/webhooks/mailgun`, the Go server matches the recipient address, extracts the verification link from email body, executes `megatools reg --scripted --verify`, and stores the validated account into `accounts.csv`, SQLite, and JSON.
3. **Keep-Alive Monitor & Manual Execution**: User views when each account was last verified and logged into. User can trigger an immediate manual keep-alive cycle or let the background timer run automatically.
4. **Data Management & Export**: User can download `accounts.csv` or `accounts.json`, filter accounts, and inspect account status.
5. **Language Switcher**: Dynamic language selection (English, Portuguese, Spanish) loaded from JSON translation records dynamically at runtime.

### Visual Identity & Theme
- **Theme**: High-contrast, clean dark-mode developer console aesthetic with subtle slate/neutral zinc tones and vibrant accents (emerald for active/verified, amber for pending verification, rose for errors, cyan for system operations).
- **Typography**: Clean monospace font for logs, credentials, and email addresses (`JetBrains Mono` / `ui-monospace`); refined sans-serif for UI chrome (`Inter` / system sans).
- **Layout Rhythm**: Header bar with dynamic language switcher, quick stats grid (4 cards), two-column layout featuring Live Action Log & Generation Trigger on the left, and Searchable Account Table on the right.

---

## 3. Key Product Decisions & Trade-Offs

- **Pure Go HTTP Server with Embedded Frontend**:
  - *Chosen Approach*: Use Go's standard library `net/http` with `embed.FS` to bundle HTML/JS/CSS directly into the binary.
  - *Why*: Enables single-binary deployment without external node_modules or separate frontend web server, perfect for `scratch` container.
- **Mailgun Inbound Webhook**:
  - *Chosen Approach*: Handle standard Mailgun inbound parsed email webhooks (multipart/form-data and JSON payloads with `recipient`, `body-plain`, `stripped-text`, `timestamp`, `token`, `signature`).
  - *Why*: Allows instantaneous verification without polling loops or Mail.tm rate limits. Also includes a simulation/test webhook endpoint in the dashboard for local development when a public webhook URL is not yet bound.
- **Megatools Integration in Docker Scratch**:
  - *Chosen Approach*: In Dockerfile multi-stage build, download/extract official static `megatools` binary (`megatools-*-linux-x86_64`) and copy it along with root CA certificates (`/etc/ssl/certs/ca-certificates.crt`) into `FROM scratch`.
  - *Why*: `megatools` contains complex proprietary crypto for MEGA's SRP/RSA handshake. Bundling the static binary in `scratch` guarantees 100% compatibility with MEGA protocol while keeping the final Docker image tiny (~30MB total).
- **Storage Engine**:
  - *Chosen Approach*: SQLite file with WAL mode enabled for zero concurrency locks + synchronized append to `accounts.csv` for 100% backward compatibility with legacy scripts + JSON export endpoint.
- **Dynamic Language Strings**:
  - *Chosen Approach*: Dynamic translation map fetched from `/api/i18n?lang=...` with client-side reactive rendering, avoiding hardcoded fixed strings.

---

## 4. Technical Architecture & Data Strategy

### System Architecture Diagram

```
┌────────────────────────────────────────────────────────────────────────┐
│                        Go Application Architecture                     │
└────────────────────────────────────────────────────────────────────────┘

    Incoming Mailgun Webhook                     Web Dashboard & REST API
              │                                             │
              ▼                                             ▼
  ┌───────────────────────┐                    ┌─────────────────────────┐
  │ /api/webhooks/mailgun │                    │ Embedded Web UI / API   │
  │ - Signature Check     │                    │ - Dynamic i18n Strings  │
  │ - Extract Link        │                    │ - Account Management    │
  └───────────┬───────────┘                    │ - Manual Trigger        │
              │                                └────────────┬────────────┘
              ▼                                             │
  ┌─────────────────────────────────────────────────────────┴────────────┐
  │                        Mega Account Controller                       │
  │ - Pattern Formatter: fmt.Sprintf(pattern, counter)                   │
  │ - Pending Registration Tracker (Mutex Map)                           │
  │ - Megatools Runner (Static Binary Execution)                         │
  └───────────┬─────────────────────────────────────────────┬────────────┘
              │                                             │
              ▼                                             ▼
  ┌───────────────────────┐                    ┌─────────────────────────┐
  │ Background Ticker     │                    │ Persistence Layer       │
  │ - Runs every N days   │                    │ - accounts.csv (legacy) │
  │ - Low CPU/RAM idle    │                    │ - accounts.db (SQLite)  │
  │ - Auto Login & Status │                    │ - accounts.json export  │
  └───────────────────────┘                    └─────────────────────────┘
```

### Component Breakdown
1. `cmd/server/main.go`: Entry point, environment configuration loader, signal handler, and service startup.
2. `pkg/config/config.go`: Environment variables parsing (`PORT`, `EMAIL_PATTERN`, `EMAIL_DOMAIN`, `SIGNIN_INTERVAL_DAYS`, `MAILGUN_API_KEY`, `MEGATOOLS_PATH`, `DATA_DIR`).
3. `pkg/mega/client.go`: Megatools wrapper for `reg --scripted --register`, `reg --scripted --verify`, and `ls -u ... -p ...` for login keep-alive checks.
4. `pkg/mailgun/webhook.go`: Webhook receiver, HMAC SHA256 signature verification, and email body regex parser for verification URLs.
5. `pkg/storage/store.go`: Dual persistence handling SQLite database and standard CSV file writer with mutex synchronization.
6. `pkg/scheduler/scheduler.go`: Goroutine ticker that sleeps for configurable intervals and triggers periodic keep-alive logins sequentially with configurable delays.
7. `pkg/i18n/i18n.go`: Localization dictionary engine serving dynamic localized strings.
8. `web/`: Embedded dashboard files (`index.html`, `app.js`, `style.css`) with real-time SSE or poll-based live status updates.
9. `Dockerfile`: Multi-stage build (Builder: `golang:alpine` + curl static megatools -> Target: `scratch` with CA certificates, static megatools, and compiled Go binary).
10. `.github/workflows/docker.yml`: GitHub Actions workflow that checks out code, sets up Docker Buildx, and builds the scratch image on release or push.

---

## 5. Verification & Testing Plan

1. **Compilation Check**: Build the Go project using `go build -o mega-manager ./cmd/server` to confirm type safety and zero compilation warnings.
2. **Config Validation**: Verify default values when environment variables are unset (`EMAIL_PATTERN=mega%d`, `SIGNIN_INTERVAL_DAYS=30`).
3. **Mailgun Webhook Verification**: Send simulated Mailgun payloads to `/api/webhooks/mailgun` and verify link extraction and error handling.
4. **Storage Sync**: Verify that newly registered accounts are written to both `accounts.csv` in the exact original format and SQLite.
5. **Docker Multi-Stage Verification**: Validate `Dockerfile` syntax and scratch structure.
