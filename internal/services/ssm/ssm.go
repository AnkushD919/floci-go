package ssm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	ssmTargetPrefix = "AmazonSSM"
)

type Parameter struct {
	Name             string `json:"Name"`
	Type             string `json:"Type"`
	Value            string `json:"Value"`
	Version          int64  `json:"Version"`
	LastModifiedDate float64 `json:"LastModifiedDate"`
	ARN              string `json:"ARN"`
}

type ParameterMetadata struct {
	Name             string  `json:"Name"`
	Type             string  `json:"Type"`
	LastModifiedDate float64 `json:"LastModifiedDate"`
	LastModifiedUser string  `json:"LastModifiedUser"`
	Version          int64   `json:"Version"`
}

type PutParameterInput struct {
	Name      string `json:"Name"`
	Value     string `json:"Value"`
	Type      string `json:"Type"`
	Overwrite bool   `json:"Overwrite"`
}

type PutParameterOutput struct {
	Version int64 `json:"Version"`
	Tier    string `json:"Tier"`
}

type GetParameterInput struct {
	Name           string `json:"Name"`
	WithDecryption bool   `json:"WithDecryption"`
}

type GetParameterOutput struct {
	Parameter Parameter `json:"Parameter"`
}

type GetParametersInput struct {
	Names          []string `json:"Names"`
	WithDecryption bool     `json:"WithDecryption"`
}

type GetParametersOutput struct {
	Parameters        []Parameter `json:"Parameters"`
	InvalidParameters []string    `json:"InvalidParameters"`
}

type DescribeParametersInput struct {
	MaxResults int    `json:"MaxResults"`
	NextToken  string `json:"NextToken"`
}

type DescribeParametersOutput struct {
	Parameters []ParameterMetadata `json:"Parameters"`
	NextToken  string              `json:"NextToken,omitempty"`
}

type GetParametersByPathInput struct {
	Path           string `json:"Path"`
	Recursive      bool   `json:"Recursive"`
	WithDecryption bool   `json:"WithDecryption"`
}

type GetParametersByPathOutput struct {
	Parameters []Parameter `json:"Parameters"`
	NextToken  string      `json:"NextToken,omitempty"`
}

type DeleteParameterInput struct {
	Name string `json:"Name"`
}

type DeleteParameterOutput struct {
}

type SSMHandler struct {
	mu         sync.RWMutex
	parameters map[string]Parameter
	AccountID  string
}

func (h *SSMHandler) Name() string {
	return "ssm"
}

func (h *SSMHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 && (strings.Contains(strings.ToLower(parts[0]), "awssimplesystemsmanagement") || strings.Contains(strings.ToLower(parts[0]), "ssm")) {
			return true
		}
	}
	return false
}

func NewHandler() *SSMHandler {
	return &SSMHandler{
		parameters: make(map[string]Parameter),
		AccountID:  "000000000000",
	}
}

func (h *SSMHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	action := parts[1]

	switch action {
	case "PutParameter":
		h.handlePutParameter(w, r)
	case "GetParameter":
		h.handleGetParameter(w, r)
	case "GetParameters":
		h.handleGetParameters(w, r)
	case "DescribeParameters":
		h.handleDescribeParameters(w, r)
	case "GetParametersByPath":
		h.handleGetParametersByPath(w, r)
	case "DeleteParameter":
		h.handleDeleteParameter(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteJSONResponse(w, ssmTargetPrefix)
	}
}

func (h *SSMHandler) handlePutParameter(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input PutParameterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	if input.Name == "" || input.Value == "" || input.Type == "" {
		awserr.New(400, "ValidationException", "Name, Value, and Type are required parameters.").WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	existing, exists := h.parameters[input.Name]
	var version int64 = 1

	if exists {
		if !input.Overwrite {
			awserr.New(400, "ParameterAlreadyExists", fmt.Sprintf("The parameter %s already exists.", input.Name)).
				WriteJSONResponse(w, ssmTargetPrefix)
			return
		}
		version = existing.Version + 1
	}

	p := Parameter{
		Name:             input.Name,
		Type:             input.Type,
		Value:            input.Value,
		Version:          version,
		LastModifiedDate: float64(time.Now().Unix()),
		ARN:              fmt.Sprintf("arn:aws:ssm:us-east-1:%s:parameter%s", h.AccountID, input.Name),
	}

	h.parameters[input.Name] = p

	writeJSON(w, PutParameterOutput{
		Version: version,
		Tier:    "Standard",
	})
}

func (h *SSMHandler) handleGetParameter(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GetParameterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	p, exists := h.parameters[input.Name]
	if !exists {
		awserr.New(400, "ParameterNotFound", fmt.Sprintf("Parameter %s not found.", input.Name)).
			WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	writeJSON(w, GetParameterOutput{Parameter: p})
}

func (h *SSMHandler) handleGetParameters(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GetParametersInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	params := make([]Parameter, 0)
	invalid := make([]string, 0)

	for _, name := range input.Names {
		p, exists := h.parameters[name]
		if exists {
			params = append(params, p)
		} else {
			invalid = append(invalid, name)
		}
	}

	writeJSON(w, GetParametersOutput{
		Parameters:        params,
		InvalidParameters: invalid,
	})
}

func (h *SSMHandler) handleDescribeParameters(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input DescribeParametersInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// Ignore decode errors since fields are optional in DescribeParameters
		input = DescribeParametersInput{}
	}

	metaList := make([]ParameterMetadata, 0, len(h.parameters))
	for _, p := range h.parameters {
		metaList = append(metaList, ParameterMetadata{
			Name:             p.Name,
			Type:             p.Type,
			LastModifiedDate: p.LastModifiedDate,
			LastModifiedUser: "System",
			Version:          p.Version,
		})
	}

	writeJSON(w, DescribeParametersOutput{
		Parameters: metaList,
	})
}

func (h *SSMHandler) handleGetParametersByPath(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GetParametersByPathInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	path := input.Path
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	params := make([]Parameter, 0)

	for name, p := range h.parameters {
		if strings.HasPrefix(name, path) {
			remaining := strings.TrimPrefix(name, path)
			// Non-recursive: parameter name must not contain another slash
			if !input.Recursive && strings.Contains(remaining, "/") {
				continue
			}
			params = append(params, p)
		}
	}

	writeJSON(w, GetParametersByPathOutput{
		Parameters: params,
	})
}

func (h *SSMHandler) handleDeleteParameter(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input DeleteParameterInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	_, exists := h.parameters[input.Name]
	if !exists {
		awserr.New(400, "ParameterNotFound", fmt.Sprintf("Parameter %s not found.", input.Name)).
			WriteJSONResponse(w, ssmTargetPrefix)
		return
	}

	delete(h.parameters, input.Name)

	writeJSON(w, DeleteParameterOutput{})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *SSMHandler) GetParameters() []Parameter {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]Parameter, 0, len(h.parameters))
	for _, p := range h.parameters {
		res = append(res, p)
	}
	return res
}
