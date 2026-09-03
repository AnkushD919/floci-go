package kms

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	kmsTargetPrefix = "TrentService"
)

type KeyMetadata struct {
	KeyId        string  `json:"KeyId"`
	ARN          string  `json:"ARN"`
	Description  string  `json:"Description,omitempty"`
	Enabled      bool    `json:"Enabled"`
	CreationDate float64 `json:"CreationDate"`
}

type CreateKeyInput struct {
	Description string `json:"Description,omitempty"`
}

type CreateKeyOutput struct {
	KeyMetadata KeyMetadata `json:"KeyMetadata"`
}

type DescribeKeyInput struct {
	KeyId string `json:"KeyId"`
}

type DescribeKeyOutput struct {
	KeyMetadata KeyMetadata `json:"KeyMetadata"`
}

type KeyListEntry struct {
	KeyId  string `json:"KeyId"`
	KeyArn string `json:"KeyArn"`
}

type ListKeysInput struct {
	Limit  int    `json:"Limit"`
	Marker string `json:"Marker"`
}

type ListKeysOutput struct {
	Keys      []KeyListEntry `json:"Keys"`
	Truncated  bool           `json:"Truncated"`
}

type EncryptInput struct {
	KeyId     string `json:"KeyId"`
	Plaintext []byte `json:"Plaintext"`
}

type EncryptOutput struct {
	CiphertextBlob []byte `json:"CiphertextBlob"`
	KeyId          string `json:"KeyId"`
}

type DecryptInput struct {
	CiphertextBlob []byte `json:"CiphertextBlob"`
	KeyId          string `json:"KeyId,omitempty"`
}

type DecryptOutput struct {
	Plaintext []byte `json:"Plaintext"`
	KeyId     string `json:"KeyId"`
}

type GenerateDataKeyInput struct {
	KeyId   string `json:"KeyId"`
	KeySpec string `json:"KeySpec"`
}

type GenerateDataKeyOutput struct {
	CiphertextBlob []byte `json:"CiphertextBlob"`
	Plaintext      []byte `json:"Plaintext"`
	KeyId          string `json:"KeyId"`
}

type KMSHandler struct {
	mu        sync.RWMutex
	keys      map[string]KeyMetadata
	AccountID string
}

func (h *KMSHandler) Name() string {
	return "kms"
}

func (h *KMSHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 && (strings.Contains(strings.ToLower(parts[0]), "kms") || strings.Contains(strings.ToLower(parts[0]), "trent")) {
			return true
		}
	}
	return false
}

func NewHandler() *KMSHandler {
	return &KMSHandler{
		keys:      make(map[string]KeyMetadata),
		AccountID: "000000000000",
	}
}

func (h *KMSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	action := parts[1]

	switch action {
	case "CreateKey":
		h.handleCreateKey(w, r)
	case "DescribeKey":
		h.handleDescribeKey(w, r)
	case "ListKeys":
		h.handleListKeys(w, r)
	case "Encrypt":
		h.handleEncrypt(w, r)
	case "Decrypt":
		h.handleDecrypt(w, r)
	case "GenerateDataKey":
		h.handleGenerateDataKey(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteJSONResponse(w, kmsTargetPrefix)
	}
}

func (h *KMSHandler) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input CreateKeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		input = CreateKeyInput{}
	}

	keyUuid := generateUUID()
	arn := fmt.Sprintf("arn:aws:kms:us-east-1:%s:key/%s", h.AccountID, keyUuid)

	metadata := KeyMetadata{
		KeyId:        keyUuid,
		ARN:          arn,
		Description:  input.Description,
		Enabled:      true,
		CreationDate: float64(time.Now().Unix()),
	}

	h.keys[keyUuid] = metadata
	h.keys[arn] = metadata

	writeJSON(w, CreateKeyOutput{KeyMetadata: metadata})
}

func (h *KMSHandler) handleDescribeKey(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input DescribeKeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	metadata, exists := h.keys[input.KeyId]
	if !exists {
		awserr.New(400, "NotFoundException", fmt.Sprintf("Key %s not found.", input.KeyId)).
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	writeJSON(w, DescribeKeyOutput{KeyMetadata: metadata})
}

func (h *KMSHandler) handleListKeys(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	keysList := make([]KeyListEntry, 0)
	seen := make(map[string]bool)

	for _, meta := range h.keys {
		if !seen[meta.KeyId] {
			seen[meta.KeyId] = true
			keysList = append(keysList, KeyListEntry{
				KeyId:  meta.KeyId,
				KeyArn: meta.ARN,
			})
		}
	}

	writeJSON(w, ListKeysOutput{
		Keys:      keysList,
		Truncated: false,
	})
}

func (h *KMSHandler) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input EncryptInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	// Verify key exists
	if _, exists := h.keys[input.KeyId]; !exists {
		awserr.New(400, "NotFoundException", fmt.Sprintf("Key %s not found.", input.KeyId)).
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	// Stateless format: MOCK-KMS:<keyId>:<plaintext>
	prefix := []byte(fmt.Sprintf("MOCK-KMS:%s:", input.KeyId))
	ciphertext := append(prefix, input.Plaintext...)

	writeJSON(w, EncryptOutput{
		CiphertextBlob: ciphertext,
		KeyId:          input.KeyId,
	})
}

func (h *KMSHandler) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input DecryptInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	cipherStr := string(input.CiphertextBlob)
	if !strings.HasPrefix(cipherStr, "MOCK-KMS:") {
		awserr.New(400, "InvalidCiphertextException", "The ciphertext is invalid or cannot be decrypted.").
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	parts := strings.SplitN(cipherStr, ":", 3)
	if len(parts) < 3 {
		awserr.New(400, "InvalidCiphertextException", "The ciphertext is invalid or cannot be decrypted.").
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	keyId := parts[1]
	plaintext := []byte(parts[2])

	writeJSON(w, DecryptOutput{
		Plaintext: plaintext,
		KeyId:     keyId,
	})
}

func (h *KMSHandler) handleGenerateDataKey(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GenerateDataKeyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	// Verify key exists
	if _, exists := h.keys[input.KeyId]; !exists {
		awserr.New(400, "NotFoundException", fmt.Sprintf("Key %s not found.", input.KeyId)).
			WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	keySize := 32 // Default for AES_256
	if input.KeySpec == "AES_128" {
		keySize = 16
	}

	plaintextKey := make([]byte, keySize)
	if _, err := rand.Read(plaintextKey); err != nil {
		awserr.New(500, "InternalFailure", err.Error()).WriteJSONResponse(w, kmsTargetPrefix)
		return
	}

	prefix := []byte(fmt.Sprintf("MOCK-KMS:%s:", input.KeyId))
	ciphertextKey := append(prefix, plaintextKey...)

	writeJSON(w, GenerateDataKeyOutput{
		CiphertextBlob: ciphertextKey,
		Plaintext:      plaintextKey,
		KeyId:          input.KeyId,
	})
}

func generateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	// uuid format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *KMSHandler) GetKeys() []KeyMetadata {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]KeyMetadata, 0, len(h.keys))
	for _, k := range h.keys {
		res = append(res, k)
	}
	return res
}
