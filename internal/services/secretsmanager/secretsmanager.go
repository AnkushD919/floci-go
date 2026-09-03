package secretsmanager

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
	secretsmanagerTargetPrefix = "secretsmanager"
)

type Secret struct {
	ARN          string  `json:"ARN"`
	Name         string  `json:"Name"`
	SecretString string  `json:"SecretString,omitempty"`
	CreatedDate  float64 `json:"CreatedDate"`
	Description  string  `json:"Description,omitempty"`
}

type CreateSecretInput struct {
	Name         string `json:"Name"`
	SecretString string `json:"SecretString,omitempty"`
	Description  string `json:"Description,omitempty"`
}

type CreateSecretOutput struct {
	ARN  string `json:"ARN"`
	Name string `json:"Name"`
}

type GetSecretValueInput struct {
	SecretId string `json:"SecretId"`
}

type GetSecretValueOutput struct {
	ARN          string  `json:"ARN"`
	Name         string  `json:"Name"`
	SecretString string  `json:"SecretString,omitempty"`
	CreatedDate  float64 `json:"CreatedDate"`
}

type DescribeSecretInput struct {
	SecretId string `json:"SecretId"`
}

type DescribeSecretOutput struct {
	ARN         string `json:"ARN"`
	Name        string `json:"Name"`
	Description string `json:"Description,omitempty"`
}

type ListSecretsInput struct {
	MaxResults int    `json:"MaxResults"`
	NextToken  string `json:"NextToken"`
}

type ListSecretsOutput struct {
	SecretList []Secret `json:"SecretList"`
	NextToken  string   `json:"NextToken,omitempty"`
}

type PutSecretValueInput struct {
	SecretId     string `json:"SecretId"`
	SecretString string `json:"SecretString,omitempty"`
}

type PutSecretValueOutput struct {
	ARN  string `json:"ARN"`
	Name string `json:"Name"`
}

type DeleteSecretInput struct {
	SecretId                   string `json:"SecretId"`
	ForceDeleteWithoutRecovery bool   `json:"ForceDeleteWithoutRecovery"`
}

type DeleteSecretOutput struct {
	ARN          string  `json:"ARN"`
	Name         string  `json:"Name"`
	DeletionDate float64 `json:"DeletionDate"`
}

type SecretsManagerHandler struct {
	mu        sync.RWMutex
	secrets   map[string]Secret
	nameToARN map[string]string
	AccountID string
}

func (h *SecretsManagerHandler) Name() string {
	return "secretsmanager"
}

func (h *SecretsManagerHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "secretsmanager") {
			return true
		}
	}
	return false
}

func NewHandler() *SecretsManagerHandler {
	return &SecretsManagerHandler{
		secrets:   make(map[string]Secret),
		nameToARN: make(map[string]string),
		AccountID: "000000000000",
	}
}

func (h *SecretsManagerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	action := parts[1]

	switch action {
	case "CreateSecret":
		h.handleCreateSecret(w, r)
	case "GetSecretValue":
		h.handleGetSecretValue(w, r)
	case "DescribeSecret":
		h.handleDescribeSecret(w, r)
	case "ListSecrets":
		h.handleListSecrets(w, r)
	case "PutSecretValue":
		h.handlePutSecretValue(w, r)
	case "DeleteSecret":
		h.handleDeleteSecret(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
	}
}

func (h *SecretsManagerHandler) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input CreateSecretInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	if input.Name == "" {
		awserr.New(400, "ValidationException", "Name is a required parameter.").WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	if _, exists := h.nameToARN[input.Name]; exists {
		awserr.New(400, "ResourceExistsException", fmt.Sprintf("A secret with the name %s already exists.", input.Name)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	arn := fmt.Sprintf("arn:aws:secretsmanager:us-east-1:%s:secret:%s-mockedrandom", h.AccountID, input.Name)
	secret := Secret{
		ARN:          arn,
		Name:         input.Name,
		SecretString: input.SecretString,
		CreatedDate:  float64(time.Now().Unix()),
		Description:  input.Description,
	}

	h.secrets[arn] = secret
	h.nameToARN[input.Name] = arn

	writeJSON(w, CreateSecretOutput{
		ARN:  arn,
		Name: input.Name,
	})
}

func (h *SecretsManagerHandler) handleGetSecretValue(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GetSecretValueInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	arn, found := h.resolveSecretARN(input.SecretId)
	if !found {
		awserr.New(400, "ResourceNotFoundException", fmt.Sprintf("Secrets Manager can't find the specified secret: %s", input.SecretId)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	secret := h.secrets[arn]
	writeJSON(w, GetSecretValueOutput{
		ARN:          secret.ARN,
		Name:         secret.Name,
		SecretString: secret.SecretString,
		CreatedDate:  secret.CreatedDate,
	})
}

func (h *SecretsManagerHandler) handleDescribeSecret(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input DescribeSecretInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	arn, found := h.resolveSecretARN(input.SecretId)
	if !found {
		awserr.New(400, "ResourceNotFoundException", fmt.Sprintf("Secrets Manager can't find the specified secret: %s", input.SecretId)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	secret := h.secrets[arn]
	writeJSON(w, DescribeSecretOutput{
		ARN:         secret.ARN,
		Name:        secret.Name,
		Description: secret.Description,
	})
}

func (h *SecretsManagerHandler) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input ListSecretsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// Ignore list filters for simplicity
	}

	secretList := make([]Secret, 0, len(h.secrets))
	for _, secret := range h.secrets {
		secretList = append(secretList, secret)
	}

	writeJSON(w, ListSecretsOutput{
		SecretList: secretList,
	})
}

func (h *SecretsManagerHandler) handlePutSecretValue(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input PutSecretValueInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	arn, found := h.resolveSecretARN(input.SecretId)
	if !found {
		awserr.New(400, "ResourceNotFoundException", fmt.Sprintf("Secrets Manager can't find the specified secret: %s", input.SecretId)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	secret := h.secrets[arn]
	secret.SecretString = input.SecretString
	h.secrets[arn] = secret

	writeJSON(w, PutSecretValueOutput{
		ARN:  secret.ARN,
		Name: secret.Name,
	})
}

func (h *SecretsManagerHandler) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input DeleteSecretInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	arn, found := h.resolveSecretARN(input.SecretId)
	if !found {
		awserr.New(400, "ResourceNotFoundException", fmt.Sprintf("Secrets Manager can't find the specified secret: %s", input.SecretId)).
			WriteJSONResponse(w, secretsmanagerTargetPrefix)
		return
	}

	secret := h.secrets[arn]

	delete(h.secrets, arn)
	delete(h.nameToARN, secret.Name)

	writeJSON(w, DeleteSecretOutput{
		ARN:          secret.ARN,
		Name:         secret.Name,
		DeletionDate: float64(time.Now().Unix()),
	})
}

func (h *SecretsManagerHandler) resolveSecretARN(secretId string) (string, bool) {
	// 1. Try directly as ARN
	if _, exists := h.secrets[secretId]; exists {
		return secretId, true
	}
	// 2. Try as name
	if arn, exists := h.nameToARN[secretId]; exists {
		return arn, true
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *SecretsManagerHandler) GetSecrets() []Secret {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]Secret, 0, len(h.secrets))
	for _, s := range h.secrets {
		res = append(res, s)
	}
	return res
}
