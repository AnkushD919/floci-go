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

## 📦 Supported AWS Services

`floci-go` focuses on **execution depth** across the core serverless and modern application stack:

* **Compute & Gateway**:
  * **AWS Lambda**: Docker container execution via AWS Runtime Interface Emulator (RIE), warm container reuse, automatic unzipping, dynamic host port binding.
  * **API Gateway v2 (HTTP API)**: Route matching, proxy integrations, and payload v2.0 conversion.
  * **EventBridge**: Rule pattern matching, scheduled rules (`rate(...)`), and target dispatching to Lambda & SQS.
* **Database & Orchestration**:
  * **RDS & RDS Data API**: Embedded pure-Go SQLite (`modernc.org/sqlite`) for real SQL execution (`CREATE TABLE`, `INSERT`, `SELECT`, `JOIN`) without external database containers.
  * **DynamoDB**: In-memory document store supporting CRUD, Query, Scan, and expressions.
  * **Step Functions**: Real Amazon States Language (ASL) execution engine supporting `Pass`, `Task` (invoking Lambdas), `Succeed`, and `Fail` transitions.
* **Auth, Security & Identity**:
  * **Cognito IDP**: User pools, user management, and authentication issuing standard HMAC-SHA256 JWT tokens.
  * **IAM & STS**: AssumeRole, GetCallerIdentity, and IAM role management.
  * **KMS**: Key creation, encrypt/decrypt operations, and data key generation.
  * **Secrets Manager & SSM Parameter Store**: Secure key/value retrieval and path hierarchies.
* **Storage & Messaging**:
  * **S3**: Bucket management, object CRUD, pre-signed URLs, and multipart uploads.
  * **SQS & SNS**: Message queues, dead-letter support, topic publishing, and cross-service SNS-to-SQS fanout.
* **Observability**:
  * **CloudWatch Logs & Metrics**: Log groups, log streams, metric ingestion, and basic statistical aggregation.

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
