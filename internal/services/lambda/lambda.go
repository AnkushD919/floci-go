package lambda

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

type LambdaFunction struct {
	FunctionName string
	Runtime      string
	Handler      string
	Code         []byte
	ImageURI     string
	Description  string
	Timeout      int
	MemorySize   int
	Environment  map[string]string
	ARN          string
	Role         string

	// Container state
	ContainerName string
	ContainerID   string
	HostPort      int
	mu            sync.Mutex
}

type LambdaHandler struct {
	mu        sync.RWMutex
	functions map[string]*LambdaFunction
	AccountID string
}

func NewHandler() *LambdaHandler {
	// Stop any lingering floci lambda containers on startup
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "for /f %i in ('docker ps -a --filter name=floci-lambda- -q') do docker rm -f %i").Run()
	} else {
		exec.Command("bash", "-c", "docker ps -a --filter name=floci-lambda- -q | xargs -r docker rm -f").Run()
	}

	return &LambdaHandler{
		functions: make(map[string]*LambdaFunction),
		AccountID: "000000000000",
	}
}

func (h *LambdaHandler) Name() string {
	return "lambda"
}

func (h *LambdaHandler) Matches(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/2015-03-31/functions")
}

// Request and Response JSON types
type FunctionCode struct {
	ZipFile         []byte `json:"ZipFile,omitempty"`
	S3Bucket        string `json:"S3Bucket,omitempty"`
	S3Key           string `json:"S3Key,omitempty"`
	S3ObjectVersion string `json:"S3ObjectVersion,omitempty"`
	ImageUri        string `json:"ImageUri,omitempty"`
}

type Environment struct {
	Variables map[string]string `json:"Variables"`
}

type CreateFunctionInput struct {
	FunctionName string       `json:"FunctionName"`
	Runtime      string       `json:"Runtime"`
	Role         string       `json:"Role"`
	Handler      string       `json:"Handler"`
	Code         FunctionCode `json:"Code"`
	Description  string       `json:"Description"`
	Timeout      int          `json:"Timeout"`
	MemorySize   int          `json:"MemorySize"`
	Environment  *Environment `json:"Environment"`
	PackageType  string       `json:"PackageType"`
}

type FunctionConfiguration struct {
	FunctionName string       `json:"FunctionName"`
	FunctionArn  string       `json:"FunctionArn"`
	Runtime      string       `json:"Runtime"`
	Role         string       `json:"Role"`
	Handler      string       `json:"Handler"`
	CodeSize     int64        `json:"CodeSize"`
	Description  string       `json:"Description"`
	Timeout      int          `json:"Timeout"`
	MemorySize   int          `json:"MemorySize"`
	LastModified string       `json:"LastModified"`
	CodeSha256   string       `json:"CodeSha256"`
	Version      string       `json:"Version"`
	Environment  *Environment `json:"Environment,omitempty"`
	PackageType  string       `json:"PackageType"`
}

type ListFunctionsOutput struct {
	Functions []FunctionConfiguration `json:"Functions"`
}

func (h *LambdaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Split path parts: /2015-03-31/functions -> ["", "2015-03-31", "functions"]
	parts := strings.Split(path, "/")

	if len(parts) == 3 {
		// POST /2015-03-31/functions (CreateFunction)
		// GET /2015-03-31/functions (ListFunctions)
		if r.Method == http.MethodPost {
			h.handleCreateFunction(w, r)
		} else if r.Method == http.MethodGet {
			h.handleListFunctions(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) >= 4 {
		funcName := parts[3]
		subAction := ""
		if len(parts) >= 5 {
			subAction = parts[4]
		}

		if subAction == "invocations" && r.Method == http.MethodPost {
			h.handleInvokeFunction(w, r, funcName)
			return
		}

		if subAction == "code" && r.Method == http.MethodPut {
			h.handleUpdateFunctionCode(w, r, funcName)
			return
		}

		if subAction == "" {
			if r.Method == http.MethodGet {
				h.handleGetFunction(w, r, funcName)
			} else if r.Method == http.MethodDelete {
				h.handleDeleteFunction(w, r, funcName)
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func (h *LambdaHandler) handleCreateFunction(w http.ResponseWriter, r *http.Request) {
	var input CreateFunctionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.functions[input.FunctionName]; exists {
		awserr.New(409, "ResourceConflictException", fmt.Sprintf("Function already exists: %s", input.FunctionName)).
			WriteJSONResponse(w, "Lambda")
		return
	}

	timeout := input.Timeout
	if timeout == 0 {
		timeout = 3
	}
	memory := input.MemorySize
	if memory == 0 {
		memory = 128
	}

	envVars := make(map[string]string)
	if input.Environment != nil && input.Environment.Variables != nil {
		envVars = input.Environment.Variables
	}

	arn := fmt.Sprintf("arn:aws:lambda:us-east-1:%s:function:%s", h.AccountID, input.FunctionName)

	fn := &LambdaFunction{
		FunctionName: input.FunctionName,
		Runtime:      input.Runtime,
		Handler:      input.Handler,
		Code:         input.Code.ZipFile,
		ImageURI:     input.Code.ImageUri,
		Description:  input.Description,
		Timeout:      timeout,
		MemorySize:   memory,
		Environment:  envVars,
		ARN:          arn,
		Role:         input.Role,
	}

	h.functions[input.FunctionName] = fn

	// Respond 201
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(h.configOf(fn))
}

func (h *LambdaHandler) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	funcs := make([]FunctionConfiguration, 0, len(h.functions))
	for _, fn := range h.functions {
		funcs = append(funcs, h.configOf(fn))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ListFunctionsOutput{Functions: funcs})
}

func (h *LambdaHandler) handleGetFunction(w http.ResponseWriter, r *http.Request, name string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fn, exists := h.functions[name]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Function not found: %s", name)).
			WriteJSONResponse(w, "Lambda")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// AWS returns both Configuration and Code location/details
	resp := map[string]interface{}{
		"Configuration": h.configOf(fn),
		"Code": map[string]interface{}{
			"RepositoryType": "S3",
			"Location":       "http://localhost:4566/mock-lambda-code-location",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *LambdaHandler) handleDeleteFunction(w http.ResponseWriter, r *http.Request, name string) {
	h.mu.Lock()
	fn, exists := h.functions[name]
	if !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Function not found: %s", name)).
			WriteJSONResponse(w, "Lambda")
		return
	}
	delete(h.functions, name)
	h.mu.Unlock()

	// Stop running container
	fn.stopContainer()

	w.WriteHeader(http.StatusNoContent)
}

func (h *LambdaHandler) handleUpdateFunctionCode(w http.ResponseWriter, r *http.Request, name string) {
	var input FunctionCode
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	fn, exists := h.functions[name]
	if !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Function not found: %s", name)).
			WriteJSONResponse(w, "Lambda")
		return
	}

	fn.Code = input.ZipFile
	fn.ImageURI = input.ImageUri
	h.mu.Unlock()

	// Force stop old container to refresh code/image on next invoke
	fn.stopContainer()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.configOf(fn))
}

func (h *LambdaHandler) handleInvokeFunction(w http.ResponseWriter, r *http.Request, name string) {
	h.mu.RLock()
	fn, exists := h.functions[name]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Function not found: %s", name)).
			WriteJSONResponse(w, "Lambda")
		return
	}

	// Read invoke payload (event body)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure container is running (starts if warm-pool container doesn't exist or stopped)
	if err := fn.startContainer(); err != nil {
		awserr.New(500, "ECRImagePullFailedException", fmt.Sprintf("Failed to spawn docker container: %v", err)).
			WriteJSONResponse(w, "Lambda")
		return
	}

	// Send invoke POST request to Runtime Interface Emulator (RIE) inside container
	rieURL := fmt.Sprintf("http://localhost:%d/2015-03-31/functions/function/invocations", fn.HostPort)
	req, err := http.NewRequest(http.MethodPost, rieURL, bytes.NewReader(payload))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(fn.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		awserr.New(502, "SubprocessInvocationFailed", fmt.Sprintf("Failed to contact RIE container: %v", err)).
			WriteJSONResponse(w, "Lambda")
		return
	}
	defer resp.Body.Close()

	// Forward response headers (e.g. X-Amz-Function-Error) and body
	for k, vv := range resp.Header {
		if strings.HasPrefix(k, "X-Amz-") {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *LambdaHandler) configOf(fn *LambdaFunction) FunctionConfiguration {
	sha := sha256.Sum256(fn.Code)
	shaBase64 := base64.StdEncoding.EncodeToString(sha[:])

	env := &Environment{}
	if len(fn.Environment) > 0 {
		env.Variables = fn.Environment
	}

	return FunctionConfiguration{
		FunctionName: fn.FunctionName,
		FunctionArn:  fn.ARN,
		Runtime:      fn.Runtime,
		Role:         fn.Role,
		Handler:      fn.Handler,
		CodeSize:     int64(len(fn.Code)),
		Description:  fn.Description,
		Timeout:      fn.Timeout,
		MemorySize:   fn.MemorySize,
		LastModified: time.Now().UTC().Format(time.RFC3339),
		CodeSha256:   shaBase64,
		Version:      "$LATEST",
		Environment:  env,
		PackageType:  "Zip",
	}
}

func (fn *LambdaFunction) startContainer() error {
	fn.mu.Lock()
	defer fn.mu.Unlock()

	if fn.ContainerID != "" {
		// Verify container running
		cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", fn.ContainerID)
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		// Clean up dead container reference
		exec.Command("docker", "rm", "-f", fn.ContainerID).Run()
		fn.ContainerID = ""
	}

	port, err := getFreePort()
	if err != nil {
		return fmt.Errorf("failed to allocate free port: %w", err)
	}
	fn.HostPort = port

	containerName := fmt.Sprintf("floci-lambda-%s-%d", fn.FunctionName, time.Now().UnixNano())
	fn.ContainerName = containerName

	var image string
	if fn.ImageURI != "" {
		image = fn.ImageURI
	} else {
		image = getDockerImageForRuntime(fn.Runtime)
	}

	args := []string{"run", "-d", "--name", containerName, "-p", fmt.Sprintf("%d:8080", port)}

	for k, v := range fn.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if fn.ImageURI == "" {
		hostDir := filepath.Join(os.TempDir(), fmt.Sprintf("floci-lambda-%s", fn.FunctionName))
		_ = os.RemoveAll(hostDir)
		_ = os.MkdirAll(hostDir, 0o750)

		if err := unzipBytes(fn.Code, hostDir); err != nil {
			return fmt.Errorf("failed to unzip code bytes: %w", err)
		}

		args = append(args, "-v", fmt.Sprintf("%s:/var/task:ro", filepath.Clean(hostDir)))
		args = append(args, "-e", fmt.Sprintf("_HANDLER=%s", fn.Handler))
	}

	args = append(args, image)

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run docker command: %s (%w)", string(out), err)
	}

	fn.ContainerID = strings.TrimSpace(string(out))

	// Poll RIE status (wait for container startup)
	ready := false
	for i := 0; i < 300; i++ {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		exec.Command("docker", "kill", fn.ContainerID).Run()
		exec.Command("docker", "rm", "-f", fn.ContainerID).Run()
		fn.ContainerID = ""
		return fmt.Errorf("timeout waiting for container RIE to respond on port %d", port)
	}

	return nil
}

func (fn *LambdaFunction) stopContainer() {
	fn.mu.Lock()
	defer fn.mu.Unlock()

	if fn.ContainerID != "" {
		exec.Command("docker", "kill", fn.ContainerID).Run()
		exec.Command("docker", "rm", "-f", fn.ContainerID).Run()
		fn.ContainerID = ""
	}
	hostDir := filepath.Join(os.TempDir(), fmt.Sprintf("floci-lambda-%s", fn.FunctionName))
	_ = os.RemoveAll(hostDir)
}

func getDockerImageForRuntime(runtime string) string {
	switch {
	case strings.HasPrefix(runtime, "nodejs"):
		v := strings.TrimPrefix(runtime, "nodejs")
		v = strings.TrimSuffix(v, ".x")
		return "public.ecr.aws/lambda/nodejs:" + v
	case strings.HasPrefix(runtime, "python"):
		v := strings.TrimPrefix(runtime, "python")
		return "public.ecr.aws/lambda/python:" + v
	case strings.HasPrefix(runtime, "go"):
		return "public.ecr.aws/lambda/provided:al2023"
	case strings.HasPrefix(runtime, "provided"):
		return "public.ecr.aws/lambda/provided:al2023"
	case strings.HasPrefix(runtime, "java"):
		v := strings.TrimPrefix(runtime, "java")
		return "public.ecr.aws/lambda/java:" + v
	case strings.HasPrefix(runtime, "dotnet"):
		v := strings.TrimPrefix(runtime, "dotnet")
		return "public.ecr.aws/lambda/dotnet:" + v
	case strings.HasPrefix(runtime, "ruby"):
		v := strings.TrimPrefix(runtime, "ruby")
		return "public.ecr.aws/lambda/ruby:" + v
	default:
		return "public.ecr.aws/lambda/provided:al2023"
	}
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func unzipBytes(zipContent []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(zipContent), int64(len(zipContent)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, 0o750)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0o750); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// GetFunctions returns the snapshot list for Console UI integration
func (h *LambdaHandler) GetFunctions() []FunctionConfiguration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]FunctionConfiguration, 0, len(h.functions))
	for _, fn := range h.functions {
		res = append(res, h.configOf(fn))
	}
	return res
}
