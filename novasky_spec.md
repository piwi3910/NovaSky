# NovaSky Architecture & Build Spec

## Overview

NovaSky is an observatory conditions platform combining: - All-sky
camera system - Sensor fusion (rain, humidity, wind, etc.) -
Vision-based sky analysis - Policy engine (safe/unsafe + imaging
quality) - NINA integration via Alpaca (SafetyMonitor +
ObservingConditions)

## Core Principles

-   Message bus centric (Redis + BullMQ)
-   Edge-first safety (RP5 independent)
-   Distributed-ready (NAS offload)
-   Modular services
-   Driver-agnostic (INDI optional)

## High-Level Architecture

\[Ingestion\] -\> \[Redis/BullMQ\] -\> \[Workers\] -\> \[Policy\] -\>
\[Outputs/UI\]

## Tech Stack

### Core

-   Node.js + TypeScript
-   Fastify (API)
-   Redis
-   BullMQ

### Frontend

-   React + Vite
-   Mantine UI
-   WebSocket

### Storage

-   SQLite (MVP)
-   Postgres (future)
-   Local disk spool

### Optional

-   Python (advanced CV)
-   Go (edge ingest / watchdog)

## Services

-   novasky-ingest-camera
-   novasky-ingest-sensors
-   novasky-worker-vision-basic
-   novasky-worker-vision-advanced
-   novasky-policy
-   novasky-alpaca
-   novasky-api
-   novasky-ui
-   novasky-alerts

## Queues (BullMQ)

-   frames.analysis.basic
-   frames.analysis.advanced
-   policy.evaluate
-   storage.archive
-   timelapse.render
-   alerts.dispatch

## Event Model

### Example

{ id, type, source, timestamp, payload }

## Safety Model

States: - SAFE - UNSAFE - UNKNOWN

Triggers: - Rain - Clouds (threshold) - Sensor fault

## Imaging Quality

-   EXCELLENT
-   GOOD
-   POOR
-   UNUSABLE

## NINA Integration

Expose Alpaca endpoints: - /safetymonitor - /observingconditions

## Deployment

### RP5 (MVP)

All services local

### Future

RP5: - ingest - policy - safety

NAS: - workers - storage - UI

## Repo Structure

apps/ packages/ workers/

## Future Enhancements

-   ML cloud detection
-   Seeing estimation
-   Multi-camera
-   Fleet management

## Key Rule

Hard safety must NEVER depend on external systems.
