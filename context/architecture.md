# floci-go Architecture

This document describes the high-level architecture of `floci-go`, a lightweight, high-performance local AWS emulator written in Go.

```
                  ┌────────────────────────────────────────┐
                  │          AWS SDK / CLI Client          │
                  └───────────────────┬────────────────────┘
                                      │
                                      ▼ (Port :4566)
                  ┌────────────────────────────────────────┐
                  │            HTTP Multiplexer            │
                  │       (net/http + chi Router)          │
                  └───────────────────┬────────────────────┘
                                      │
                                      ▼
                  ┌────────────────────────────────────────┐
                  │          Protocol Dispatcher           │
                  │   Detects: Host, Header, Path, Query   │
                  └─────────┬─────────┬──────────┬─────────┘
                            │         │          │
         ┌──────────────────┘         │          └───────────────────┐
         ▼ (Query)                    ▼ (JSON 1.1)                   ▼ (REST-JSON/XML)
┌──────────────────┐        ┌──────────────────┐        ┌──────────────────┐
│  Query Protocol  │        │  JSON Protocol   │        │  REST Protocol   │
│    Deserializer  │        │    Deserializer  │        │    Deserializer  │
└────────┬─────────┘        └────────┬─────────┘        └────────┬─────────┘
         │                           │                           │
         └─────────────────┐         │         ┌─────────────────┘
                           ▼         ▼         ▼
                  ┌────────────────────────────────────────┐
                  │            Service Registry            │
                  │ (Dispatches to specific AWS Service)   │
                  └───────────────────┬────────────────────┘
                                      │
                                      ▼
                  ┌────────────────────────────────────────┐
                  │          Service Handler/Logic         │
                  │     (e.g., S3, DynamoDB, SQS, STS)     │
                  └─────────┬──────────────────┬───────────┘
                            │                  │
                            ▼ (Stateless/State)▼ (Container Workloads)
                  ┌──────────────────┐┌──────────────────┐
                  │ Storage Backend  ││  Docker Backend  │
                  │ (Memory / Bolt)  ││ (Docker Engine)  │
                  └──────────────────┘└──────────────────┘
```

## Core Components

### 1. HTTP Multiplexer & Protocol Router
Everything goes through a single entry point (typically port `4566`). The router inspects incoming requests to match them against their targeted AWS services and protocols:
- **`Host` Header**: Subdomain matching (e.g. `my-bucket.s3.localhost.localstack.cloud` or `sqs.us-east-1.amazonaws.com`).
- **`X-Amz-Target` Header**: Present in JSON 1.1 services (e.g., `DynamoDB_20120810.CreateTable`).
- **Request Path**: API Gateway routes, S3 object paths, REST endpoints.
- **Form/URL Action parameter**: Query-based protocols (like SQS/SNS) specify `Action=CreateQueue` in the body or query parameters.

### 2. Protocol Handlers (`internal/protocol`)
AWS uses four main wire protocols:
- **Query (XML-based)**: Used by SQS, SNS, IAM, STS, CloudFormation. Inputs are typically URL-encoded form parameters. Outputs are XML.
- **JSON 1.1 / 1.0**: Used by DynamoDB, Kinesis, KMS, Secrets Manager. Input and outputs are JSON. Request target is specified via the `X-Amz-Target` header.
- **REST-JSON**: Used by Lambda, API Gateway, Cognito, Bedrock. URI path variables carry structural data; HTTP payloads are JSON.
- **REST-XML**: Used by S3. Path parameters and XML payloads.

### 3. Service Registry & Handlers (`internal/services`)
Each AWS service is structured as its own package inside `internal/services/`.
- Every service handler implements `http.Handler` or defines explicit handlers mapped to the protocol.
- Logic is kept thin and delegates state management to the Storage Backend.
- Error behaviors must map exactly to AWS counterparts using custom `AwsError` types.

### 4. Storage Abstractions (`internal/store`)
To support memory-only, hybrid, and persistent modes, all services persist state using a unified Storage engine.
- **`Store` Interface**: Simple key-value/object methods.
- **Memory Engine**: Fast, concurrent-safe map implementations.
- **Persistent Engine**: BoltDB (`bbolt`) backend for single-binary native persistence without external database engines.

### 5. Container & Sidecar Workloads (`internal/docker`)
For heavy services (Lambda runtimes, RDS engines, MSK/Kafka, Neptune/Gremlin):
- Floci uses the Docker API to pull, start, stop, and manage life cycles of official image bases.
- S3 features an optional native integration with a DuckDB sidecar for Athena queries.
