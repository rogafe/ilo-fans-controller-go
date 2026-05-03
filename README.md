# iLO Fans Controller (Go)

A self-hosted web application to monitor server temperatures and control HPE iLO fan behavior using Redfish, SSH, and SNMP.

## What It Does

This project provides a browser UI and HTTP API to:

- Read current fan speeds from iLO Redfish.
- Apply manual fan speed changes through iLO SSH commands.
- Read temperature sensors through SNMP (with Redfish metadata enrichment when available).
- Manage named fan presets stored in Postgres.
- Manage and apply advanced fan-control profiles stored in Postgres.
- Stream command feedback to clients via WebSocket console events.

## Safety & Risk Warnings

This tool can directly change thermal behavior. Incorrect settings can cause overheating, instability, hardware throttling, or shutdown.
This project was vibe coded and is provided without guarantees; use it entirely at your own risk.

- Use at your own risk on hardware you understand and actively monitor.
- Test changes incrementally and watch temperatures under real load.
- Advanced profiles may disable or relax thermal safeguards.
- Built-in advanced profiles include high-risk settings and are intended for experienced users only.
- Advanced profile settings applied to iLO are not persistent across iLO or server reboots.

Recommended operating practice:

1. Start with conservative fan values.
2. Apply one change at a time.
3. Observe temperatures for several minutes under load.
4. Keep out-of-band recovery access available.

## Features

- Go backend (Fiber + GORM + Postgres).
- Server-rendered UI plus static frontend assets (Tailwind + DaisyUI + Alpine).
- Real-time console feedback over WebSocket.
- Built-in default presets (`Silent Mode`, `Normal Mode`, `Turbo Mode`).
- Built-in advanced profiles seeded on first run.
- Safety constraints through configurable minimum fan speed and apply-time tolerance/timeout.

## Architecture at a Glance

Runtime dependencies and protocol usage:

- Redfish (`https://<ILO_HOST>/redfish/v1/chassis/1/Thermal`) for fan and temperature metadata.
- SSH to iLO for fan-setting and advanced profile command application.
- SNMP for temperature polling.
- Postgres for presets and advanced profile persistence.

Core components:

- `main.go`: app bootstrap, config load, DB open, service/handler wiring.
- `internals/config`: environment parsing and validation.
- `internals/services/ilo`: Redfish, SSH, SNMP integration.
- `internals/services/presets`: preset persistence and default seeding.
- `internals/services/advancedprofiles`: advanced profile persistence, seeding, lookup.
- `internals/handlers` + `internals/router`: HTTP and WebSocket endpoints.

## Prerequisites

- Go `1.24+`
- Node.js `20+`
- `pnpm` (project uses `pnpm@10`)
- Docker + Docker Compose (recommended for local Postgres)
- Access to an HPE iLO endpoint with:
  - Redfish API enabled
  - SSH access enabled
  - SNMP enabled (for temperatures API)

Optional but used by Task workflow:

- `task` (Taskfile runner)
- `air` (hot reload for Go dev server)

## Configuration

Use environment variables. In `ENV=dev`, the app loads `.env` automatically.

Never commit real credentials. Rotate any exposed iLO or DB secrets immediately.

### Server

- `PORT` (default: `3000`)
- `ENV` (example: `dev` to load `.env`)

### Database

Choose one of:

1. `DATABASE_URL`
2. `DB_*` parts (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`)

Notes:

- If `DATABASE_URL` is empty, it is built from `DB_*` values.
- `DB_USER` and `DB_NAME` are required when using `DB_*` mode.

### iLO Authentication

- `ILO_HOST`
- `ILO_USERNAME`
- `ILO_PASSWORD`

### SSH Compatibility Overrides

Use these when your iLO firmware requires legacy/explicit algorithm settings:

- `ILO_SSH_KEX_ALGORITHMS` (comma-separated)
- `ILO_SSH_HOST_KEY_ALGORITHMS` (comma-separated)
- `ILO_SSH_PUBKEY_ACCEPTED_ALGORITHMS` (comma-separated)
- `ILO_SSH_CIPHERS` (comma-separated)
- `ILO_SSH_MACS` (comma-separated)

### SNMP

- `ILO_SNMP_HOST` (defaults to `ILO_HOST`)
- `ILO_SNMP_PORT` (default: `161`)
- `ILO_SNMP_COMMUNITY` (default: `public`)
- `ILO_SNMP_VERSION` (default: `2c`)
- `ILO_SNMP_TIMEOUT_SECONDS` (default: `5`)
- `ILO_SNMP_RETRIES` (default: `1`)

### Safety Controls

- `MINIMUM_FAN_SPEED` (default: `10`, valid range: `0..100`)
- `FAN_APPLY_TIMEOUT_SECONDS` (default: `30`, valid range: `1..600`)
- `FAN_APPLY_TOLERANCE` (default: `2`, valid range: `0..20`)
- `ILO_INSECURE_TLS` (default: `true`)

## Quick Start (Local Development)

1. Install dependencies:

```bash
go mod download
pnpm install
```

2. Create `.env` (example):

```bash
PORT=3001
ENV=dev

DB_HOST=127.0.0.1
DB_PORT=5433
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=ilo_fans_controller
DB_SSLMODE=disable

MINIMUM_FAN_SPEED=10
FAN_APPLY_TIMEOUT_SECONDS=30
FAN_APPLY_TOLERANCE=2
ILO_INSECURE_TLS=true

ILO_HOST=<your-ilo-host>
ILO_USERNAME=<your-ilo-user>
ILO_PASSWORD=<your-ilo-password>

ILO_SSH_KEX_ALGORITHMS=diffie-hellman-group14-sha1
ILO_SSH_HOST_KEY_ALGORITHMS=ssh-rsa
ILO_SSH_PUBKEY_ACCEPTED_ALGORITHMS=ssh-rsa
ILO_SSH_CIPHERS=aes256-ctr
ILO_SSH_MACS=hmac-sha2-256

ILO_SNMP_HOST=<your-ilo-host>
ILO_SNMP_PORT=161
ILO_SNMP_COMMUNITY=public
ILO_SNMP_VERSION=2c
ILO_SNMP_TIMEOUT_SECONDS=5
ILO_SNMP_RETRIES=1
```

3. Start development stack:

```bash
task dev
```

This runs:

- `task docker:up` (Postgres via docker-compose)
- `task dev:go` (Go server via `air`)
- `task dev:frontend` (asset watch build)

4. Open the app:

- `http://localhost:<PORT>`

Useful task commands:

```bash
task dev:go
task dev:frontend
task docker:up
task docker:down
task docker:logs
task build
```

## Quick Start (Docker)

1. Build image:

```bash
docker build -t ilo-fans-controller-go:local .
```

2. Start Postgres dependency:

```bash
docker compose up -d
```

3. Run app container:

```bash
docker run --rm -p 3000:3000 \
  --env-file .env \
  ilo-fans-controller-go:local
```

The Docker image exposes port `3000` and runs with `ENV=production` by default.

## Running Tests

Run all tests:

```bash
go test ./...
```

## Core API Endpoints

### UI and Console

- `GET /` -> HTML UI
- `GET /ws/console?client_id=<id>` -> WebSocket event stream

### Fans and Temperatures

- `GET /api/fans`
- `POST /api/fans`
- `GET /api/temperatures`

### Presets

- `GET /api/presets`
- `POST /api/presets`

### Advanced Profiles

- `GET /api/advanced-profiles`
- `POST /api/advanced-profiles`
- `POST /api/advanced-profiles/apply`

### Example Payloads

`POST /api/fans` (`SetFansRequest`)

```json
{
  "clientId": "browser-1",
  "speed": 35,
  "fans": {
    "Fan 1": 35,
    "Fan 2": 40
  }
}
```

`POST /api/advanced-profiles/apply` (`ApplyAdvancedProfileRequest`)

```json
{
  "clientId": "browser-1",
  "profileName": "Conservative",
  "confirmation": "APPLY ADVANCED PROFILE"
}
```

`POST /api/presets` (array of preset objects)

```json
[
  { "name": "Night", "speeds": [20] },
  { "name": "Day", "speeds": [45] }
]
```

`POST /api/advanced-profiles` (array of profile objects)

```json
[
  {
    "name": "My Custom Profile",
    "warning": "Custom thermal profile.",
    "builtIn": false,
    "commandBundle": {
      "fanMinimums": [{ "fan": 1, "value": 12 }],
      "fanMaximums": [{ "fan": 1, "value": 60 }],
      "pidLows": [{ "sensors": [33, 34], "value": 3000 }],
      "pidHighs": [{ "sensors": [53], "value": 3000 }],
      "ocsd": [{ "sensors": [24], "value": 2 }],
      "disabledSensors": [45]
    }
  }
]
```

Notes:

- `builtIn: true` advanced profiles are read-only on save.
- Preset speeds must respect configured fan speed constraints.
- For temporary iLO outages, fans/profile apply endpoints may return `503` with `Retry-After: 5`.

## Troubleshooting

### `400 iLO credentials are not configured`

Set `ILO_HOST`, `ILO_USERNAME`, and `ILO_PASSWORD`.

### `400 iLO SNMP is not configured`

Set `ILO_SNMP_HOST` and `ILO_SNMP_COMMUNITY` (and verify SNMP is enabled on iLO).

### `503 iLO is unreachable`

The app returns a temporary-unavailable response when iLO is down/unreachable and includes `Retry-After: 5` on key fan/profile operations. Restore network/power/iLO availability, then retry.

### Preset save fails due to speed validation

Ensure each speed is between configured minimum and `100`, where minimum is `MINIMUM_FAN_SPEED`.

### Advanced profile save/apply errors

Common causes:

- Unknown profile name when applying.
- Attempt to overwrite built-in profile (`builtIn=true`).
- Invalid command bundle values.
- Missing/incorrect confirmation string for apply request.

### TLS/SSH algorithm handshake issues

Some iLO firmware requires explicit legacy SSH algorithm settings. Configure `ILO_SSH_*` variables to match supported algorithms.

### Database configuration errors at startup

Provide either:

- `DATABASE_URL`, or
- valid `DB_*` (`DB_USER` and `DB_NAME` required)

## Development Notes

- Presets and advanced profiles are auto-migrated with GORM at startup.
- Default presets and advanced profiles are seeded only when tables are empty.
- Frontend assets are built into `assets/dist`.
- Go server serves static assets from `/assets`.

## License

No license file is currently present in this repository.

If you plan to distribute or reuse this project, add an explicit license file (for example `LICENSE`) to define usage terms.
