# floci-go AI Development Workflow

This document outlines instructions and guidelines for AI coding assistants working on the `floci-go` codebase.

## Workflow Rules

### 1. Test-Driven Implementation
Before adding code for a new AWS operation:
- Review the corresponding tests in the `compatibility-tests/` directories (e.g., `sdk-test-go/tests/*_test.go` or BATS tests).
- Run the compatibility suite targeting that service to observe current behavior and failures.
- Implement only what is required to pass the test cases, preventing over-engineering.

### 2. AWS Protocol Conformity
AWS client libraries (SDKs) are extremely picky about headers, XML namespace tags, and JSON schemas.
* **JSON 1.1**: Responses must serialize type metadata correctly (e.g., in DynamoDB: `{"Item": {"pk": {"S": "my-key"}}}`). Always include headers like `x-amz-crc32` and correct error structures.
* **Query Protocol**: Always match XML response fields, including namespaces, root elements, and error layouts. 
* **REST Protocols**: Ensure HTTP headers are set exactly as expected (e.g. S3 requires `ETag`, `x-amz-request-id`, and correct `Content-Length`).

### 3. Exact Error Emulation
If an operation should fail (e.g., trying to read a non-existent parameter from SSM), the handler must return the exact status code and payload format expected by the AWS SDK.
- Use `internal/awserr` templates.
- Consult AWS documentation or the existing Java source for the precise error codes.

### 4. Incremental Architecture Integration
When implementing a new service:
1. **Define Types**: Model the request and response structures matching AWS schemas.
2. **Register Route**: Add the service parser/router mappings inside `internal/router/router.go`.
3. **Draft Handler**: Implement the handler using the corresponding protocol unmarshaller.
4. **Implement State/Storage**: Access storage under regional namespace rules.
5. **Verify Compatibility**: Run the Go tests:
   ```bash
   cd compatibility-tests/sdk-test-go && go test -run TestYourService -v
   ```

### 5. Code Structure Guidelines
- **Thin Handlers**: Handlers should only parse protocol input, execute validation, delegate to storage/services, and encode outputs. Keep actual business logic out of router middleware.
- **Pure Go**: Avoid importing CGO dependencies unless absolutely necessary.
- **Concurrent-Safe**: Always use appropriate locking (`sync.RWMutex`) or thread-safe primitives when modifying in-memory storage elements.
