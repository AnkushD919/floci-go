# floci-go Request & Lifecycle Flow

This document details the lifecycle of a request in `floci-go`, illustrating how AWS clients interact with the emulator.

## Request Lifecycle Flowchart

```
┌──────────┐
│  Client  │ (e.g. AWS CLI: aws s3 ls)
└────┬─────┘
     │
     │ 1. HTTP Request (S3 Protocol / Port 4566)
     ▼
┌───────────────────────┐
│ net/http Port Listener│
└────┬──────────────────┘
     │
     │ 2. Route Request to Protocol Matcher
     ▼
┌─────────────────────────────────────────────────────────────┐
│ Protocol Multiplexer / Middleware                           │
│ - Inspect Host: s3.localhost                                │
│ - Inspect Headers: X-Amz-Target, Content-Type               │
│ - Inspect Path: /bucket-name/key                            │
└────┬────────────────────────────────────────────────────────┘
     │
     │ 3. Match S3 (REST-XML Protocol) -> Handled by internal/protocol/restxml
     ▼
┌─────────────────────────────────────────────────────────────┐
│ REST-XML Protocol Unmarshaler                               │
│ - Extracts path variables (Bucket, Key)                     │
│ - Unmarshals request XML payload (if any) to Go struct      │
└────┬────────────────────────────────────────────────────────┘
     │
     │ 4. Route to S3 Service Logic (internal/services/s3)
     ▼
┌─────────────────────────────────────────────────────────────┐
│ S3 Service Handler                                          │
│ - Validates parameters                                      │
│ - If error, raises AwsError (e.g., NoSuchBucket)            │
│ - Delegates object retrieval/storage to Storage Backend     │
└────┬────────────────────────────────────────────────────────┘
     │
     │ 5. Perform storage operation
     ▼
┌──────────────────────────┐
│     Storage Backend      │ (Memory map or BoltDB persistent file)
└────┬─────────────────────┘
     │
     │ 6. Return Go objects / status
     ▼
┌─────────────────────────────────────────────────────────────┐
│ REST-XML Protocol Marshaller                                │
│ - Encodes S3 Go structs into AWS-compatible XML response    │
│ - Sets HTTP Status (e.g. 200 OK, 204 No Content)            │
│ - Serializes required headers (x-amz-request-id, ETag...)   │
└────┬────────────────────────────────────────────────────────┘
     │
     │ 7. Return HTTP Response
     ▼
┌──────────┐
│  Client  │
└──────────┘
```

## Detailed Lifecycle Steps

### Step 1: Client Dispatch
The client (AWS SDK, terraform, aws-cli, etc.) points to `http://localhost:4566`.
```bash
aws --endpoint-url=http://localhost:4566 s3 mb s3://my-test-bucket
```

### Step 2: Protocol Identification
The router inspects the incoming `*http.Request` to identify the service target:
1. **Target Header**: If `X-Amz-Target` matches `DynamoDB_20120810.CreateTable`, route is DynamoDB.
2. **Host Header**: If Host matches `*.s3.localhost.localstack.cloud` or `s3.amazonaws.com`, route is S3.
3. **HTTP Method & Path**: If request is `POST /` with header `Content-Type: application/x-www-form-urlencoded` and body contains `Action=CreateQueue`, route is SQS.

### Step 3: Unmarshaling
The matched service unmarshals input:
* **Query services (SQS, SNS)** parse URL values into structural Go parameter fields.
* **JSON services (DynamoDB)** parse JSON payload to corresponding struct schema mapping.
* **REST-XML (S3)** extracts bucket and object from URL route params and deserializes XML payload if body is present.

### Step 4: Logic & Validation
The service performs standard validation (e.g. validation patterns for bucket name lengths, SQS queue name restrictions). Any discrepancy returns an immediate standard AWS Protocol Error structure:
```xml
<ErrorResponse xmlns="http://queue.amazonaws.com/doc/2012-11-05/">
    <Error>
        <Type>Sender</Type>
        <Code>InvalidParameterValue</Code>
        <Message>Value my_invalid_queue for parameter QueueName is invalid.</Message>
    </Error>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
</ErrorResponse>
```

### Step 5: Backend Access
State changes are written to the `Store` interface.

### Step 6: Response Marshalling
The response payload is serialized using the wire format defined by the service API (XML, JSON, or empty-body with specific headers). Crucial AWS headers like `x-amz-request-id`, `x-amz-id-2`, and `Content-Type` are correctly formatted.
