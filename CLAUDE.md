# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

NovaSky is an all-sky camera observatory platform built as Go microservices communicating via Redis Streams. It captures sky images via INDI, processes them through a pipeline (debayer, stretch, detection, overlay), evaluates safety conditions, and exposes results through a React dashboard and NINA-compatible Alpaca API.

## Architecture

Microservices architecture. Each service is an independent Go binary. Services communicate ONLY through Redis Streams — never direct calls. Any service can run on any host pointed at the same Redis + PostgreSQL.

**Pipeline flow:**
```
ingest-camera → frames.raw → processing → frames.detection → detection
                                        → frames.overlay   → overlay
                                        → frames.export    → export
                                        → frames.timelapse  → timelapse
                                        → policy.evaluate   → policy → alerts
```

**Key constraint:** Hard safety must NEVER depend on external systems. The RP5 with camera must operate independently.

## Tech Stack

- **Backend:** Go, Redis Streams, PostgreSQL (GORM)
- **Frontend:** React + Vite + Mantine UI
- **Camera:** INDI protocol via pure Go client (no Python)
- **Plate solving:** ASTAP with H18 catalog
- **No Python** unless absolutely no alternative exists

## Repo Structure

```
cmd/           # Go service entry points (one dir per service)
internal/      # Shared Go packages (db, redis, config, indi, models)
web/           # React frontend (Vite + Mantine)
```

## Development Workflow

- Write code locally on Mac
- `git commit` and `git push`
- SSH to RP5 (192.168.10.170), `git pull`, build and run there
- Never run Go services or tests locally — all execution on the RP5

## Build & Run Commands (on RP5)

```bash
# Build all services
cd ~/NovaSky && go build ./cmd/...

# Build a single service
go build -o bin/novasky-api ./cmd/novasky-api

# Run a single service
DATABASE_URL=postgres://novasky:novasky_dev@localhost:5432/novasky \
REDIS_URL=localhost:6379 \
./bin/novasky-api

# Run tests
go test ./internal/...
go test ./cmd/novasky-api/...

# Run a specific test
go test -run TestAutoExposure ./internal/autoexposure/

# Frontend (React)
cd web && pnpm install && pnpm dev
```

## Key Design Decisions

- **16-bit raw capture** — camera configured for ASI_IMG_RAW16 via INDI
- **Hot config reload** — config changes via Redis pub/sub, only restart affected components. INDI server only restarts on driver/device change.
- **Backpressure** — capture throttles/pauses when processing queue depth > 3
- **Auto-exposure** — three-phase: slew (fast convergence), track (PID within 2% buffer), predict (trend-based twilight handling)
- **Overlays as metadata** — stored as JSON, rendered client-side in UI. Original images never modified.
- **Processing in Go** — debayer, stretch, SCNR, noise reduction all implemented in Go (image stdlib + custom), not Python

## Dev Host

- **RP5:** 192.168.10.170, user `piwi`
- ZWO ASI676MC camera connected via USB
- INDI built from source at `/usr/local/bin/`
- PostgreSQL, Redis, GPSD running as system services
