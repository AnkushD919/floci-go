# floci-go Architectural Decisions

This document outlines key technical decisions made for `floci-go` to meet the objective of building a fast, lightweight, and low-footprint local AWS emulator.

## Decisions

### 1. HTTP Router: `net/http` Standard Library + `chi`
* **Context**: We need to route requests using varied attributes (Host header, path, custom headers, query/body variables).
* **Options**: Standard `net/http` mux, `chi`, `gin`, `fiber` (fasthttp).
* **Decision**: We use the standard library `net/http` for handlers, wrapped with `go-chi/chi` for routing.
* **Consequences**: 
  - Zero heavy external dependencies.
  - Trivial compatibility with standard Go middleware stacks (CORS, logging, recovery).
  - Explicit HTTP handler signatures (`http.HandlerFunc`) simplify parsing custom protocol rules.
  - Highly performant with very low resource usage compared to reflection-heavy web frameworks.

### 2. State & Storage: Memory Map & BoltDB
* **Context**: Floci requires an ephemeral (in-memory) default state for speed and CI execution, as well as a persistent profile for developer instances.
* **Options**: Redis, SQLite, BoltDB (`go.etcd.io/bbolt`), PostgreSQL.
* **Decision**: Memory maps by default. BoltDB (`bbolt`) for persistence.
* **Consequences**:
  - BoltDB is a pure-Go key-value store database that compiles directly into the application binary.
  - No external database container or binary dependency is needed for persistent runs.
  - Enables clean file-based transaction bounds matching AWS persistence requirements.
  - Zero dependencies on CGO (which SQLite often requires unless using `modernc.org/sqlite`), enabling simple cross-compilation.

### 3. DynamoDB Engine: Custom In-Memory Mapping
* **Context**: DynamoDB is complex, supporting structured expressions, transactional batches, indexes, and streams.
* **Options**: Embedded sqlite engine, custom memory maps with AST expression evaluation, or forwarding to an official `dynamodb-local` Java runtime.
* **Decision**: Implement a custom pure-Go in-memory database adapter for standard operations.
* **Consequences**:
  - Eliminates the need to run the heavy Java-based `dynamodb-local` container/sidecar, which defeats the low-memory goal (<15MB binary).
  - Faster test runs and lower container cold-starts.
  - High fidelity for CRUD, Query, Scan, and transaction endpoints by matching schemas explicitly.

### 4. Container Strategy (Lambda/RDS/MSK): Docker API integration
* **Context**: Certain services require exact runtime validation (e.g., executing actual Python/Node lambda scripts, running a real PostgreSQL engine, or Gremlin websocket servers).
* **Options**: Simulated mock engines vs Docker integration.
* **Decision**: Use Docker API integration for execution profiles that require real fidelity (RDS, Lambda, Neptune, MSK).
* **Consequences**:
  - Keeps the core binary small (~15MB).
  - Heavy logic is outsourced to the local Docker engine when requested.
  - Strict behavior validation is maintained.

### 5. Multi-Region handling
* **Context**: Clients request different regions dynamically (e.g., `us-east-1`, `eu-central-1`).
* **Decision**: Region multiplexing inside the Storage Backend.
* **Consequences**:
  - Instead of running separate servers per region, a single router port matches regional bounds (e.g. storing SQS queues under a `region/queue-name` key).
  - Substantially reduces emulator idle resources.
