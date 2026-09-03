# floci-go Code Standards

This document defines the coding standards and quality practices to be followed when developing `floci-go`.

## Guidelines

### 1. Go Idioms & Formatting
- **Standard Formatting**: All code must conform to `gofmt` and `goimports` formatting guidelines.
- **Variable Naming**: Use short, descriptive variable names (e.g. `r` for `*http.Request`, `w` for `http.ResponseWriter`, `ctx` for `context.Context`).
- **Structs**: Document complex structs. Use field tags (`json:"..."` or `xml:"..."`) matching exact AWS payload responses.

### 2. Error Handling
- **No Panics**: Do not use `panic` or `recover` for flow control. Handlers should return error values up to the handler interface.
- **AWS Errors**: Wrap errors using the local `awserr` package helpers:
  ```go
  if err != nil {
      awserr.WriteError(w, r, awserr.ErrNoSuchKey("The specified key does not exist."))
      return
  }
  ```
- **Error Wrapping**: Use standard library wrapping (`fmt.Errorf("context: %w", err)`) to preserve underlying errors for internal logging.

### 3. Concurrency & Performance
- **Mutex Usage**: Protect all shared resources. If modifying regional storage configurations, use `sync.RWMutex`. Keep the critical section as small as possible.
- **Request Contexts**: Always pass and respect request context cancellation (`r.Context()`) when executing long-running operations or DB lookups.
- **Resource Cleanup**: Ensure all network connections, directory stages, and open files are closed immediately using deferred statements (`defer file.Close()`).

### 4. Logging & Diagnostics
- **Levels**: Use structured logging containing levels: `INFO`, `DEBUG`, `WARN`, `ERROR`.
- **Target Logs**: Log incoming request metadata (Service, Action, Path, Duration) under `DEBUG` or `INFO` level to simplify emulator troubleshooting.
- **No Output to stdout in library code**: Use the central logger wrapper (`internal/logger`) instead of direct calls to `fmt.Println` or `log.Println`.

### 5. Dependency Rules
- **No Bloat**: Keep external dependencies to a bare minimum.
- **Preferred Dependencies**:
  - Routing: `github.com/go-chi/chi/v5`
  - Persistence: `go.etcd.io/bbolt`
  - Docker API Client: `github.com/docker/docker/client`
  - AWS SDK: Use only for testing (`github.com/aws/aws-sdk-go-v2/service/...`)
- Do not add web framework abstraction layers unless explicitly approved.
