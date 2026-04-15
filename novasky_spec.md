# NovaSky Architecture & Build Spec

## Overview

NovaSky is an observatory conditions platform combining:
- All-sky camera system
- Sensor fusion (rain, humidity, wind, etc.)
- Vision-based sky analysis + object detection
- Policy engine (safe/unsafe + imaging quality)
- NINA integration via Alpaca (SafetyMonitor + ObservingConditions)

## Core Principles

- Microservice architecture — each service is an independent Go binary
- Message bus centric (Redis Streams) — services communicate only through streams, never direct calls
- Edge-first safety (RP5 independent)
- Distributed-ready — any service can run on any host, just point at same Redis + Postgres
- Driver-agnostic (INDI)
- Go for all backend services
- No Python unless no alternative exists
- Hot config reload — config changes via Redis pub/sub, only restart what's affected (INDI server only restarts on driver/device change)

## High-Level Architecture

[Ingestion] -> [Redis Streams] -> [Workers] -> [Policy] -> [Outputs/UI]

## Tech Stack

### Core

- Go
- Redis Streams (message bus)
- PostgreSQL (GORM)

### Frontend

- React + Vite
- Mantine UI
- WebSocket
- Client-side overlay rendering (SVG/Canvas layer on top of JPEG)

### Camera

- INDI protocol (pure Go client, no Python)

### GPIO / Hardware

- Sensor support via GPIO/I2C/SPI (temperature, humidity, pressure, rain, wind)
- Dew heater control via GPIO PWM — configurable target delta above dew point
- RTC (Real Time Clock) support — sync system clock from hardware RTC on boot, NTP fallback

### Plate Solving

- ASTAP (`astap_cli`) — local plate solver for all-sky cameras
- H18 star catalog (downloaded during provisioning)

### Storage

- PostgreSQL
- Local disk spool
- Export folder structure: `export/{YYYY-MM-DD}/` with TIFF + JPEG per frame

## Services

- novasky-ingest-camera
- novasky-ingest-sensors
- novasky-worker-processing
- novasky-worker-detection
- novasky-worker-overlay
- novasky-worker-export
- novasky-worker-timelapse
- novasky-policy
- novasky-alpaca
- novasky-api
- novasky-ui
- novasky-alerts
- novasky-mqtt (Home Assistant integration)
- novasky-publisher (YouTube auto-publish)
- novasky-storage (remote storage sync: NFS/SMB/S3)
- novasky-stream (RTSP/HLS live stream)
- novasky-gpio (sensor reading + dew heater control via GPIO/I2C)
- novasky-grafana (embedded Grafana, optional)

## Redis Streams

- frames.raw (captured raw frames)
- frames.processing (debayer, stretch, JPEG)
- frames.detection (object detection + sky analysis)
- frames.overlay (compute overlay metadata)
- frames.export (save TIFF + JPEG to date folder)
- frames.timelapse (feed frames to timelapse/keogram/panoramic)
- policy.evaluate
- alerts.dispatch

## Event Model

### Example

{ id, type, source, timestamp, payload }

## Configuration

Stored in PostgreSQL. Changes broadcast via Redis pub/sub (`novasky:config-changed`).
Each service subscribes and hot-reloads only what it needs. INDI server only restarts on driver or device change.

### Camera Config

- INDI driver (e.g. indi_asi_ccd)
- INDI device name
- Location: latitude, longitude, elevation (manual or via GPSD)
- GPSD: optional, when enabled auto-fills location, fields become read-only

### Imaging Config (separate day/night profiles)

Day/night switch based on sun altitude (configurable twilight angle, default -6°).

#### Twilight Transition
- When crossing the twilight threshold, don't instantly jump to the other profile's gain
- Gradually ramp gain from current value toward target profile's gain over a configurable transition period
- Exposure adjusts simultaneously via auto-exposure to maintain ADU target
- Transition speed: configurable (e.g. change gain by 1 unit per capture cycle, or over N minutes)
- Prevents harsh frame-to-frame jumps at day/night boundary
- Same smooth approach for any other profile differences (stretch mode transitions can switch instantly since they're post-processing)

Per profile:
- Gain (fixed value)
- Min exposure (ms, sub-millisecond supported)
- Max exposure (ms)
- ADU target (percentage, default 30%)
- Stretch mode: none / linear / auto (for JPEG generation)

### Auto-Exposure

- Runs in ingest-camera service
- Each capture cycle: measure median ADU from raw FITS, adjust exposure toward target
- Exposure priority: adjust exposure first, boost gain only when at max exposure
- Uses sun altitude calculation for day/night profile selection
- Sub-millisecond exposure support (camera minimum ~0.032ms)

#### Startup Behavior
- On startup, load last known exposure + gain from DB (persisted each cycle)
- Resume from last values instead of starting from max exposure
- Avoids slow convergence after restarts

#### ADU Convergence (PID-style)
- Two-phase approach, similar to chrony clock discipline:
  - **Slew mode**: when ADU error > buffer zone (default 2%), apply full ratio correction to converge quickly
  - **Track mode**: when ADU within buffer zone (±2% of target), switch to PID loop with gentle adjustments
- PID in track mode: small proportional corrections damped over time to avoid frame-to-frame exposure oscillation
- Configurable buffer zone percentage (default 2%)
- Result: smooth exposure transitions, no visible flicker between consecutive frames
- Drift file: persist current exposure + gain to DB every cycle for restart recovery

#### Predictive Exposure
- Maintain rolling history of last N exposure values + ADU readings (e.g. last 10)
- Detect trend direction: exposure increasing (getting darker) or decreasing (getting brighter)
- Calculate rate of change (exposure drift per cycle)
- Apply predictive correction: pre-adjust next exposure based on trend, not just current error
- Handles twilight transitions smoothly — exposure ramps up/down ahead of the curve instead of chasing it
- Sunrise/sunset are predictable gradients, so trend prediction is highly effective here

## Processing Worker

Picks up raw FITS frames from `frames.processing` stream:
- Debayer (Bayer pattern from FITS header, e.g. RGGB)
- Dark frame subtraction (if enabled, matched from dark library)
- Bad pixel interpolation (if map exists)
- Apply processing pipeline based on profile config
- Generate JPEG preview
- Compute median ADU from raw data
- Update frame record in DB with JPEG path
- Publish to downstream streams (detection, overlay, export, timelapse)

### Processing Pipeline (configurable per day/night profile, applied in order)

- SCNR (Subtractive Chromatic Noise Reduction) — remove green cast from OSC cameras, configurable strength
- White balance — configurable R/G/B multipliers
- Stretch: none / linear / auto / adaptive / GHS
  - None: linear 16→8 bit
  - Linear: percentile stretch (configurable percentiles)
  - Auto: per-channel percentile stretch
  - Adaptive: non-linear stretch preserving star colors and faint detail
  - GHS (Generalised Hyperbolic Stretch / Arcsinh): configurable D, b, SP, HP parameters — ideal for nighttime all-sky
- Stacking: combine last N frames to increase SNR (configurable N, method: mean/median/sigma-clip)

### Noise Reduction (configurable per day/night profile)

Spatial filters (applied per frame):
- Off — no spatial filtering
- Gaussian blur — configurable kernel size, reduces random noise
- Bilateral filter — edge-preserving denoise, keeps stars sharp while smoothing sky background
- Median filter — removes salt-and-pepper noise (hot pixels), configurable kernel size

Temporal noise reduction:
- Stacking — combine last N frames to increase SNR (configurable N)
- Stacking methods: mean, median, sigma-clipped mean
- Alignment not needed (camera is fixed, no field rotation on short timescales)
- Stacked result used for JPEG preview and detection
- Individual raw frames still saved separately

Skyglow reduction:
- Background model extraction — compute smooth background gradient from non-star pixels
- Subtract background model to remove light pollution gradient
- Configurable aggressiveness (polynomial order or mesh size)
- Preserves nebulae and extended objects while removing artificial skyglow
- Per-channel background modeling to handle color gradients (e.g. sodium lamp orange)

Calibration frames:
- Dark frame subtraction (from dark library, temperature-matched)
- Flat frame correction — correct vignetting/uneven illumination across field
- Flat frame library: capture flats, store indexed, auto-apply in processing
- Bad pixel map — detect and interpolate stuck/hot pixels

## Detection Worker

Picks up frames from `frames.detection` stream. Runs analysis and stores results as structured data in DB:

### Sky Analysis
- Cloud detection — brightness/histogram analysis of sky regions
- Cloud coverage percentage
- SQM (Sky Quality Meter) — computed from background pixel values in dark sky regions
- Seeing estimation — star FWHM measurement from detected stars
- Rain detection — camera lens drop detection (frame analysis) + sensor data
- Skyglow detection — measure background brightness gradient, identify light pollution sources and direction
- Skyglow level (mag/arcsec² per region) — logged over time to track light pollution trends

### Plate Solving (core feature)
- Uses ASTAP (CLI, `astap_cli`) for local plate solving — no internet required
- Star catalog: H18 (download during provisioning)
- Solve one frame to get WCS (World Coordinate System) — pixel-to-sky mapping
- WCS provides exact RA/Dec for every pixel position
- Combined with location + time → Altitude/Azimuth → cardinal directions
- Camera is fixed, so solve once on startup (or periodically) and cache the solution
- All overlay positions (satellites, planes, constellations, grid) computed from WCS — mathematically precise
- Re-solve automatically if camera is bumped (detection of star field shift)

### Object Detection
- Stars — detect point sources, compute positions via plate solve WCS
- Planets — identify from ephemeris + WCS mapping to pixel position
- Constellations — project constellation lines via WCS from catalog RA/Dec to pixel coords
- Satellites — predict from TLE data (CelesTrak) + SGP4 propagation, project to pixel via WCS
- Planes — query tar1090 ADS-B API, convert lat/lon/alt to RA/Dec to pixel via WCS
- Meteors — frame-to-frame diff detection for fast-moving transients

All detections stored as structured JSON per frame with object type, position (x,y + RA/Dec), magnitude, and metadata.

## Overlay Worker

Picks up frames from `frames.overlay` stream. Does NOT modify the original image. Instead:
- Computes overlay metadata (positions, labels, shapes) from detection results
- Stores overlay data as JSON per frame in DB
- UI renders overlays client-side as SVG/Canvas layer on top of JPEG
- User can toggle overlay categories individually (planes, satellites, constellations, grid, compass, etc.)
- Export worker can optionally burn overlays into a separate JPEG copy

### Frame Masking
- Configurable circular mask to define the visible sky area
- UI: drag to position center, drag edge to set radius — live preview on latest frame
- Everything outside the mask is blacked out / excluded
- Mask applied in processing worker before JPEG generation
- Detection worker ignores pixels outside mask (avoids false detections from housing/horizon)
- Mask config stored in DB: center_x, center_y, radius (pixels)

### Overlay Types
- Compass / cardinal directions
- Altitude-azimuth grid lines
- Constellation outlines + labels
- Star labels (bright stars)
- Planet labels + markers
- Satellite tracks + labels (with prediction lines)
- Plane tracks + flight info (callsign, altitude, speed)
- Meteor markers
- Moon phase indicator (current phase icon + illumination %, position on frame via WCS)
- Timestamp / metadata text
- Detection annotations (bounding boxes, labels)

### Visual Overlay Editor
Interactive drag-and-drop overlay designer in the UI. Users build custom overlay layouts on top of the live frame.

Components available:
- Text labels — free text or variables (see below)
- Images — upload custom logos, icons, watermarks
- Shapes — circles, rectangles, lines
- Built-in widgets — moon phase, compass rose, mini SQM gauge, safety badge
- Any detection overlay (constellations, grid, etc.) — toggle individually

Variables (auto-populated, insertable into text components):
- `{date}`, `{time}`, `{datetime}`
- `{exposure}`, `{gain}`, `{adu}`
- `{temperature}`, `{humidity}`, `{wind}`, `{dewpoint}`
- `{sqm}`, `{cloud_cover}`, `{sky_quality}`
- `{safety_state}`, `{imaging_quality}`
- `{moon_phase}`, `{moon_illumination}`
- `{sun_altitude}`, `{mode}`
- `{camera_name}`, `{location}`

Editor features:
- Drag to position anywhere on the frame
- Resize, rotate components
- Font, color, opacity, background settings per component
- Live preview on latest frame
- Save/load overlay layouts (stored in DB)
- Multiple layouts supported (switch between them)
- Layout used by export worker when burning overlays into JPEG copies

## Export Worker

Picks up frames from `frames.export` stream:
- Saves TIFF (debayered, from raw FITS) to `export/{YYYY-MM-DD}/`
- Saves JPEG (processed) to same folder
- Optional: saves overlay-burned JPEG as separate file
- Filenames include timestamp: `novasky_{YYYYMMDD}_{HHmmss}.{ext}`

## Timelapse Worker

Picks up frames from `frames.timelapse` stream. Generates various outputs from accumulated frames:

### Outputs
- Timelapse video — standard video from sequential frames (configurable FPS, resolution)
- Keogram — vertical strip per frame composited into a single image showing sky changes over time
- Fisheye-to-panoramic — unwarp all-sky fisheye to flat panoramic projection
- Startrails — composite of all night frames showing star movement

### Triggers
- Nightly: generates all outputs at end of night (dawn)
- On-demand: via API request
- Rolling: continuous timelapse of last N hours

## Image Processing Tuner

Interactive real-time processing parameter editor in the web UI.

### Workflow
1. Select any captured frame from the frame gallery (or use the latest)
2. Processing preview panel shows the frame with current settings applied
3. Adjust any processing parameter via sliders/controls — preview updates in real-time
4. Compare: toggle between original raw and processed view
5. Save settings to day or night profile (button for each)

### Tunable Parameters (all with live preview)
- Debayer algorithm (nearest, bilinear, VNG, AHD)
- White balance R/G/B multipliers
- SCNR strength + channel
- Stretch mode + parameters:
  - Linear: black point, white point percentiles
  - Adaptive: midtones target, highlight protection
  - GHS: D (stretch factor), b (symmetry), SP (shadow protection), HP (highlight protection)
- Noise reduction: filter type, kernel size, strength
- Skyglow reduction: aggressiveness, polynomial order
- Stacking: frame count, method
- Brightness, contrast, saturation adjustments
- Histogram display (live, per-channel RGB + luminance)

### Implementation
- API endpoint: `POST /api/process-preview` — accepts frame ID + processing params, returns processed JPEG
- Processing runs server-side (same Go processing code as the worker)
- UI debounces slider changes to avoid flooding the server
- Day/night save buttons write directly to the respective imaging profile config in DB

## Focus Mode

Special mode for camera focusing. Activated from the UI.

### Behavior
- Pauses normal capture loop
- Switches to rapid capture mode (fastest possible frame rate, short exposures)
- Crops to configurable ROI (region of interest) for faster readout
- Displays live frame in UI with focus aids

### Focus Aids
- HFR (Half Flux Radius) — computed per detected star, shown as number + graph over time
- FWHM — full width half maximum of star profiles
- Star count — number of detected point sources
- Peak pixel value — brightest star intensity
- Focus graph — HFR/FWHM trend chart, goal is to minimize
- Bahtinov mask detection — detect diffraction pattern and show focus error direction (optional)
- Zoom window — click on a star to show zoomed 100% crop with profile graph

### Controls
- Exposure override (independent from auto-exposure)
- Gain override
- ROI selection (drag rectangle on frame)
- Start/stop focus mode button
- When stopped, resumes normal capture with auto-exposure

## Policy Engine

Evaluates safety state from latest detection + sensor readings:
- Cloud cover vs threshold
- Rain detected (sensor or camera)
- Sensor fault detection
- Dew point warning — calculated from temperature + humidity, warns before dew forms
- Wind gust tracking — rolling max from sensor readings
- Defaults to UNSAFE/UNKNOWN when data is missing (fail-closed)
- Publishes state changes to Redis pub/sub
- Triggers alerts on state transitions

## Alerts

Dispatches notifications on safety state changes:
- Webhook (HTTP POST)
- Telegram
- Email
- Log to DB (always on)

## Home Assistant Integration

Publish state and sensor data to MQTT for Home Assistant auto-discovery:
- Safety state (binary sensor: safe/unsafe)
- Imaging quality (sensor)
- Cloud cover percentage (sensor)
- SQM value (sensor)
- Temperature, humidity, wind, dew point (sensors)
- Camera status (connected/disconnected)
- Moon phase + illumination (sensor)
- Day/night mode (sensor)
- Current exposure + gain (sensors)
- MQTT auto-discovery topics follow HA convention (`homeassistant/sensor/novasky_*/config`)
- Configurable MQTT broker address + credentials in settings

## YouTube Auto-Publish

Publish timelapse videos automatically to YouTube:
- At end of night, upload generated timelapse to configured YouTube channel
- Uses YouTube Data API v3 with OAuth2 credentials
- Configurable: title template (with date variables), description, privacy (public/unlisted/private)
- Enable/disable per timelapse type (timelapse, keogram, startrails, panoramic)
- Upload status tracked in DB

## Remote Storage

Save exports to remote storage backends (in addition to local):
- NFS — mount point configured in settings
- SMB/CIFS — server, share, credentials in settings
- S3 — bucket, region, access key, secret key in settings
- Configurable per output type (TIFF, JPEG, timelapse, keogram)
- Sync runs after local export completes
- Retry on failure with backoff

## Safety Model

States:
- SAFE
- UNSAFE
- UNKNOWN

Triggers:
- Rain
- Clouds (threshold)
- Sensor fault
- Dew point exceeded
- Wind gust exceeded

## Imaging Quality

- EXCELLENT
- GOOD
- POOR
- UNUSABLE

## NINA Integration

Expose Alpaca endpoints:
- /safetymonitor
- /observingconditions

## API

REST API + WebSocket for the UI:
- Status (current safety, latest frame, analysis)
- Frames (list, detail, JPEG preview)
- Analysis results (detection, SQM, cloud cover history)
- Sensor readings
- Safety history
- Config (read/write with hot reload)
- Devices (INDI device listing)
- GPSD (GPS position)
- Capture (manual trigger)
- Timelapse (trigger, status, download)

WebSocket pushes:
- Safety state changes
- New processed frames
- Auto-exposure state
- Config changes
- Detection results

## Dashboard UI

### Pages
- Dashboard — latest frame with toggleable overlay layers, safety badge, cloud gauge, SQM, sensors, auto-exposure status, safety history chart
- Frames — gallery with thumbnails, click to view full + overlay toggle
- History — safety timeline, SQM trends, cloud cover over time, nightly summaries
- Timelapse — view/download generated timelapses, keograms, startrails, panoramics
- Settings > Camera — driver, device, location, GPSD toggle
- Settings > Imaging — day/night profiles, gain, exposure, ADU, stretch, twilight angle
- Settings > Detection — enable/disable detection types, thresholds
- Settings > Alerts — notification channels, configure webhooks/Telegram/email
- Settings > Export — export path, enable/disable TIFF/JPEG/overlay

## Deployment

### RP5 (MVP)

All services local

### Future

RP5: ingest-camera, ingest-sensors, policy, safety

NAS: processing, detection, overlay, export, timelapse, API, UI

## Repo Structure

```
cmd/           # Go service entry points
internal/      # Shared Go packages
web/           # React frontend
```

## GPIO / Sensor Service

Manages hardware connected to RP5 GPIO/I2C/SPI pins:

### Sensors (I2C/SPI)
- BME280 / BME680 — temperature, humidity, pressure (optional: gas/air quality)
- Rain sensor — GPIO digital input (wet/dry) or analog via ADC
- Wind speed — GPIO pulse counter from anemometer
- Wind direction — ADC from wind vane
- Ambient light sensor — for independent day/night verification
- Configurable poll interval per sensor type

### Dew Heater Control
- GPIO PWM output to MOSFET-controlled dew heater strip
- Target: maintain lens temperature at configurable delta above dew point (e.g. +3°C)
- PID control loop: reads temperature + humidity → calculates dew point → adjusts PWM duty cycle
- Safety: max duty cycle limit, timeout if sensor fails
- Status published to Redis for dashboard display

### RTC (Real Time Clock)
- Read hardware RTC (e.g. DS3231) on boot to set system clock
- Useful when no network/NTP available at remote sites
- Falls back to NTP when available
- Configurable: RTC device path

## Disk Space Management

- Monitor disk usage on frame spool and export directories
- Configurable retention policy: keep last N days, or keep under X GB
- Auto-cleanup oldest frames when threshold exceeded
- FITS frames deleted first, JPEGs kept longer (configurable)
- Never delete frames from current night

## Health Monitoring

- Each service reports heartbeat to Redis (every 30s)
- API exposes `/api/health` — per-service status (green/red/stale)
- Dashboard shows service status panel
- Alerts if a service stops reporting heartbeats
- Track service uptime and restart counts

## Uptime Tracking

- Log imaging sessions: start/stop times, frames captured per night
- Calculate clear sky hours per night (frames where sky_quality != UNUSABLE)
- Nightly summary stored in DB: total frames, clear hours, cloud cover avg, SQM avg
- Monthly/yearly statistics on dashboard

## Dark Frame Library

- Capture dark frames at various temperatures and exposure times
- Store in library indexed by temperature + exposure
- Auto-subtract matching dark from captures to reduce hot pixels and thermal noise
- Temperature matching: find closest dark within configurable tolerance (e.g. ±2°C)
- UI: trigger dark frame capture sequence, manage library

## Bad Pixel Map

- Detect stuck/hot pixels from dark frame analysis
- Store pixel map in DB
- Interpolate bad pixels in processing worker (replace with neighbor average)
- Auto-refresh map periodically or on demand

## Live Stream

- RTSP or HLS stream of latest processed frames as video feed
- Configurable frame rate and resolution
- Embeddable in external websites
- Service: `novasky-stream`

## Public Sharing Page

- Simple read-only page: latest frame + current conditions
- No login required
- Shareable URL
- Configurable: show/hide specific data (SQM, cloud cover, safety, sensors)
- Optional: custom branding (observatory name, logo)

## Astronomy Dashboard Data

### Bortle Scale
- Calculate from SQM reading (mag/arcsec²) → Bortle class 1-9
- Display on dashboard with color indicator

### Twilight & Sun Times
- Sunrise, sunset
- Civil, nautical, astronomical twilight (start/end)
- Golden hour
- Display as timeline on dashboard for current day

### Moon Data
- Rise and set times
- Current phase name + illumination %
- Next new moon / full moon dates
- Position on sky (altitude/azimuth via ephemeris)

### Clear Sky Forecast
- Pull forecast from Open-Meteo API (cloud cover, seeing, transparency)
- Display predicted vs actual cloud cover
- Tonight's forecast summary on dashboard
- Configurable forecast source

### Weather Service Integration
Pluggable weather data sources — sensor worker aggregates from all enabled sources:
- GPIO/I2C sensors (direct hardware)
- Ecowitt-compatible weather stations (local API, auto-discovery or manual IP)
- Open-Meteo API (forecast + current conditions)
- OpenWeatherMap API
- WeatherFlow Tempest API
- Custom webhook (receive JSON POSTs from any source)
- Priority-based: local sensors override API data when available
- All readings normalized to common format and stored in sensor_readings table

## Metrics & Monitoring

### Prometheus Export
- `/metrics` endpoint on the API service (Prometheus format)
- Metrics: frame capture rate, processing latency, detection counts, queue depths, service uptime
- Sensor metrics: temperature, humidity, pressure, wind, SQM, cloud cover
- Camera metrics: exposure, gain, ADU, capture duration
- Safety state as gauge (0=UNSAFE, 1=UNKNOWN, 2=SAFE)
- Disk usage, frame counts, error rates

### Embedded Grafana
- Grafana instance bundled as `novasky-grafana` service
- Pre-configured dashboards: system overview, sky conditions, camera performance, sensor trends
- Datasource auto-configured to point at Prometheus + PostgreSQL
- Accessible at `/grafana` via API reverse proxy
- Optional — can be disabled if using external Grafana

### In-App Charts (React UI)
All key data also available as charts directly in the web UI (no Grafana dependency):
- SQM over time (nightly trend)
- Cloud cover over time
- Temperature / humidity / dew point / wind
- Exposure + gain over time (auto-exposure behavior)
- Safety state timeline
- Frame capture rate
- Detection counts (stars, satellites, planes, meteors)
- Bortle scale history
- Clear sky forecast vs actual overlay
- Customizable time range: last hour, tonight, last 7 days, last 30 days, custom

## Future Enhancements

- ML cloud detection (CNN-based)
- Seeing estimation improvements (Moffat/Gaussian PSF fitting)
- Multi-camera support
- Fleet management (multiple observatory sites)

## Key Rule

Hard safety must NEVER depend on external systems.
