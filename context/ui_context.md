# floci-go UI & Dashboard Context

This document outlines the design, capabilities, and API hooks for the optional built-in local dashboard UI of `floci-go`.

## Overview
Unlike heavy enterprise cloud emulators, the `floci-go` dashboard should be extremely fast, embeddable inside the single binary, and render directly on an auxiliary port (e.g. `http://localhost:4566/dashboard` or a dedicated dashboard port).

## Key Features

### 1. Service Status Overview
A single status matrix showing which services are currently:
- **Active**: Listening for requests.
- **Idle**: Configured but not yet accessed.
- **Disabled**: Turned off via config to save resources.
- **Docker-backed**: Actively running local container workloads.

### 2. Live Request Inspector
A real-time logging visualizer to capture incoming API calls:
- Filter by AWS Service (e.g., S3, DynamoDB).
- View raw request metadata (headers, payload, path).
- View emulator response (XML/JSON payload, HTTP status code, request duration).

### 3. Resource Visualizers
* **S3 Browser**: List local buckets, inspect metadata, browse files, upload mock files, view CORS structures.
* **DynamoDB Table Viewer**: List tables, execute basic queries, examine partition keys, check current items.
* **SQS/SNS Monitor**: Inspect queued messages, check DLQ statuses, track active subscriptions.

## UI Implementation Details

### Embeddable UI
To maintain zero external dependency footprints:
- Use standard Go `embed` system (`//go:embed`) to bundle HTML/JS/CSS assets directly into the Go executable.
- The UI should be a Single Page Application (SPA) built using lightweight Vanilla CSS and JavaScript, avoiding node modules compilation steps in build pipelines.

### REST Diagnostic API
The core daemon provides diagnostic endpoints under the `_floci/` namespace:
- `GET /_floci/services`: JSON list of all configured services and status.
- `GET /_floci/requests`: Streaming SSE (Server-Sent Events) connection of live requests.
- `GET /_floci/storage`: Debug dump of BoltDB structures and current region keys.
- `POST /_floci/reset`: Purges all in-memory database states to clean state without restarting the daemon.
