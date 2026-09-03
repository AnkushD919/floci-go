# floci-go ⚡

> **A sub-10ms, ultra-lightweight AWS emulator in pure Go — with built-in Web Console, Cognito JWTs, Step Functions, and SQLite RDS.**

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/badge/release-v0.0.1-brightgreen.svg)](https://github.com/AnkushD919/floci-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docker Size](https://img.shields.io/badge/docker-~12.3MB-2496ED?style=flat&logo=docker&logoColor=white)](#quick-start)

`floci-go` is designed for developers who want **instant local development and lightning-fast CI/CD runs** without the 1.5 GB RAM footprint, 45-second boot times, or paywalled features of traditional emulators.

---

## 🚀 Why floci-go?

| Feature | LocalStack (Python) | Kumo (Go) | Original Floci (Java) | **floci-go (Pure Go)** |
| :--- | :--- | :--- | :--- | :--- |
| **Boot Time** | ~35s – 45s | < 100ms | ~1.5s – 3s | **< 10ms (Instant)** |
| **Idle Memory (RAM)** | ~1,400 MB+ | ~30 MB | ~120 MB | **~18 MB** |
| **Docker Pull Size** | ~1.2 GB | ~32 MB | ~90 MB | **~25 MB** |
| **CGO Required** | N/A | No | No | **Zero CGO (Pure Go)** |
| **Web Console UI** | 🔒 Paid / Cloud Login | ❌ None | ❌ None | **✅ Built-in on Port 4566** |
| **Cognito (Real JWTs)** | 🔒 Paid Pro | ❌ Stubs only | ⚠️ Partial | **✅ Built-in (HS256 claims)** |
| **RDS Data API** | 🔒 Paid Pro | ❌ None | ❌ None | **✅ Built-in (SQLite powered)** |
| **Step Functions ASL** | 🔒 Paid Pro | ❌ None | ⚠️ Basic | **✅ Built-in Execution Loop** |

---

## 📦 Supported AWS Services & Features

`floci-go` focuses on **execution depth** across the core serverless and modern application stack:

### 1. Compute & Gateway
* **AWS Lambda**:
  * Docker-backed container execution via AWS Runtime Interface Emulator (RIE).
  * Supports Node.js, Python, Go, Java, and custom runtimes.
  * Warm container caching / SnapStart-style container reuse.
  * Automated `.zip` unzipping into host task mounts & dynamic port allocation.
  * Non-invasive TCP port readiness probes (`net.DialTimeout`).
  * *Operations*: `CreateFunction`, `InvokeFunction`, `UpdateFunctionCode`, `ListFunctions`, `GetFunction`, `DeleteFunction`.
* **API Gateway v2 (HTTP API)**:
  * Route matching (e.g., `GET /users`, `POST /orders/{id}`).
  * Payload format version 2.0 translation.
  * Direct invocation forwarding to Lambda functions.
  * *Operations*: `CreateApi`, `CreateRoute`, `CreateIntegration`, `CreateStage`.
* **Amazon EventBridge**:
  * Custom & default event buses with `PutEvents`.
  * JSON Pattern matching engine (`source`, `detail-type`, and `detail` payload matching).
  * Background ticker goroutine supporting scheduled rules with `rate(...)` expressions.
  * Target dispatching to Lambda functions and SQS queues.
  * *Operations*: `PutEvents`, `PutRule`, `PutTargets`, `RemoveTargets`, `DeleteRule`, `DescribeRule`.

### 2. Database & Orchestration
* **Amazon RDS & RDS Data API** *(LocalStack Pro feature, free here)*:
  * Embedded **pure-Go SQLite** backend (`modernc.org/sqlite` — zero CGO, zero external Postgres/MySQL installs).
  * Real SQL execution (`CREATE TABLE`, `INSERT`, `SELECT`, `UPDATE`, `DELETE`, `JOIN`).
  * Named parameter binding (`:param`).
  * *Operations*: `CreateDBInstance`, `DescribeDBInstances`, `DeleteDBInstance`, `ExecuteStatement`.
* **AWS Step Functions** *(LocalStack Pro feature, free here)*:
  * In-process Amazon States Language (ASL) execution engine.
  * State transitions: `Pass`, `Task` (invoking Lambda functions), `Succeed`, and `Fail`.
  * Cycle-safe execution loop (1,000 step ceiling).
  * *Operations*: `CreateStateMachine`, `DescribeStateMachine`, `StartExecution`, `DescribeExecution`, `StopExecution`, `ListStateMachines`.
* **Amazon DynamoDB**:
  * In-memory NoSQL engine (no Java `dynamodb-local` container needed).
  * HASH and composite HASH+RANGE primary key indexing.
  * Key Condition Expressions for Query and Scan operations.
  * *Operations*: `CreateTable`, `DescribeTable`, `ListTables`, `DeleteTable`, `PutItem`, `GetItem`, `DeleteItem`, `Scan`, `Query`.

### 3. Auth, Security & Identity
* **Amazon Cognito IDP** *(LocalStack Pro feature, free here)*:
  * User Pools and user management.
  * `InitiateAuth` password flow issuing standard **HS256 HMAC JWTs** with valid claims (`sub`, `iss`, `exp`).
  * Constant-time password comparison (`subtle.ConstantTimeCompare`) preventing timing attacks.
  * Token expiration validation.
  * *Operations*: `CreateUserPool`, `DescribeUserPool`, `DeleteUserPool`, `AdminCreateUser`, `AdminSetUserPassword`, `InitiateAuth`, `GetUser`.
* **AWS STS**:
  * *Operations*: `GetCallerIdentity` (returns account `000000000000`), `AssumeRole`.
* **AWS IAM**:
  * Role creation with AssumeRolePolicyDocument.
  * *Operations*: `CreateRole`, `GetRole`, `ListRoles`, `DeleteRole`.
* **AWS KMS**:
  * CMK creation, describe, and list.
  * Data key generation (`GenerateDataKey`) with cryptographically secure random bytes.
  * *Operations*: `CreateKey`, `DescribeKey`, `ListKeys`, `Encrypt`, `Decrypt`, `GenerateDataKey`.
* **AWS Secrets Manager & SSM Parameter Store**:
  * String secrets and parameter hierarchy storage (`GetParametersByPath`).
  * *Operations*: `CreateSecret`, `GetSecretValue`, `PutSecretValue`, `DeleteSecret`, `PutParameter`, `GetParameter`, `GetParametersByPath`, `DeleteParameter`.

### 4. Storage, Messaging & Observability
* **Amazon S3**:
  * Path-style (`/bucket/key`) and virtual-host addressing.
  * Multipart upload lifecycle (`CreateMultipartUpload`, `UploadPart`, `CompleteMultipartUpload`, `AbortMultipartUpload`).
  * *Operations*: `CreateBucket`, `ListBuckets`, `DeleteBucket`, `PutObject`, `GetObject`, `DeleteObject`, `HeadObject`.
* **Amazon SQS & SNS**:
  * Standard queues, message batching, visibility timeouts, and queue purging.
  * Topic publishing and cross-service fanout to SQS subscribers.
  * *Operations*: `CreateQueue`, `SendMessage`, `ReceiveMessage`, `DeleteMessage`, `CreateTopic`, `Publish`, `Subscribe`.
* **Amazon CloudWatch**:
  * Log Groups & Streams ingestion (`PutLogEvents`) and querying (`GetLogEvents`, `FilterLogEvents`).
  * Metric submission (`PutMetricData`) and statistical aggregation (`GetMetricStatistics`).

### 5. Embedded Web Console Dashboard
* Served natively on `http://localhost:4566/` (exact same port as API endpoints).
* Interactive dark-mode dashboard for real-time resource inspection across all 16 services.
* Zero external web servers or static assets required (compiled via Go `embed.FS`).

---

## ⚡ Quick Start

### 1. Run via Go CLI
```bash
git clone https://github.com/AnkushD919/floci-go.git
cd floci-go
go run ./cmd/floci
```

### 2. Run via Docker
```bash
docker run -p 4566:4566 ankush919/floci-go:latest
```

### 3. Open the Built-in Console
Open your browser to:
```
http://localhost:4566/
```
The modern dark-mode console is served on the **exact same port** (`:4566`) as the API endpoints. No secondary port, no CORS configuration required.

---

## 🛠️ Usage with AWS CLI & SDKs

Point standard AWS tools to `http://localhost:4566`:

### AWS CLI
```bash
# Create an S3 Bucket & upload an object
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-bucket
aws --endpoint-url=http://localhost:4566 s3 cp app.zip s3://my-bucket/app.zip

# Create a DynamoDB table
aws --endpoint-url=http://localhost:4566 dynamodb create-table \
    --table-name Users \
    --attribute-definitions AttributeName=UserId,AttributeType=S \
    --key-schema AttributeName=UserId,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST

# Execute SQL via RDS Data API
aws --endpoint-url=http://localhost:4566 rds-data execute-statement \
    --resource-arn "arn:aws:rds:us-east-1:000000000000:cluster:mydb" \
    --secret-arn "arn:aws:secretsmanager:us-east-1:000000000000:secret:mysecret" \
    --sql "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users (name) VALUES ('Alice'); SELECT * FROM users;"
```

### AWS SDK v2 (Go)
```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
)

cfg, err := config.LoadDefaultConfig(context.TODO(),
    config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
        func(service, region string, options ...interface{}) (aws.Endpoint, error) {
            return aws.Endpoint{
                URL:           "http://localhost:4566",
                SigningRegion: "us-east-1",
            }, nil
        },
    )),
)
client := s3.NewFromConfig(cfg)
```

### Python (boto3)
```python
import boto3

s3 = boto3.client(
    "s3",
    endpoint_url="http://localhost:4566",
    aws_access_key_id="mock",
    aws_secret_access_key="mock",
    region_name="us-east-1",
)
```

---

## 🧪 CI/CD Integration (GitHub Actions)

Speed up your automated integration test workflows by replacing heavy emulator containers with `floci-go`:

```yaml
name: Test Suite
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Start floci-go
        run: |
          go run ./cmd/floci &
          # Ready in < 10ms!
          sleep 0.1

      - name: Run Integration Tests
        env:
          AWS_ENDPOINT_URL: http://localhost:4566
          AWS_DEFAULT_REGION: us-east-1
          AWS_ACCESS_KEY_ID: test
          AWS_SECRET_ACCESS_KEY: test
        run: go test -v ./...
```

---

## 🏛️ Architecture & Extensibility

`floci-go` is built on a clean, pluggable architecture. Each service implements the minimal `ServicePlugin` interface:

```go
type ServicePlugin interface {
    Name() string
    Matches(r *http.Request) bool
    http.Handler
}
```

* **No CGO**: Uses `modernc.org/sqlite` for SQLite database support. Cross-compiles natively for all OS/architecture targets.
* **Single Port Multiplexing**: S3 path/virtual-host requests, JSON 1.1 `X-Amz-Target` headers, REST routes, and the embedded Web UI all seamlessly share port `:4566`.

---

## 🤝 Contributing

Contributions are welcome! To add or extend a service:
1. Create a package in `internal/services/<service_name>/`.
2. Implement the `ServicePlugin` interface.
3. Register the handler in `cmd/floci/main.go`.
4. Run tests:
   ```bash
   go test -v ./...
   ```

---

## 📄 License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
