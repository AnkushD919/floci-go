package kms

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKMSLifecycle(t *testing.T) {
	handler := NewHandler()

	var keyId string

	// 1. Create Key
	createInput := CreateKeyInput{
		Description: "go-test key",
	}
	bodyBytes, _ := json.Marshal(createInput)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.CreateKey")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateKey status 200, got %d", resp.StatusCode)
	}

	var createOutput CreateKeyOutput
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	keyId = createOutput.KeyMetadata.KeyId
	if keyId == "" {
		t.Errorf("expected non-empty key ID")
	}

	// 2. Describe Key
	descInput := DescribeKeyInput{
		KeyId: keyId,
	}
	bodyBytes, _ = json.Marshal(descInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.DescribeKey")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DescribeKey status 200, got %d", resp.StatusCode)
	}

	// 3. List Keys
	listInput := ListKeysInput{}
	bodyBytes, _ = json.Marshal(listInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.ListKeys")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var listOutput ListKeysOutput
	_ = json.NewDecoder(resp.Body).Decode(&listOutput)
	if len(listOutput.Keys) != 1 {
		t.Errorf("expected 1 key in list, got %d", len(listOutput.Keys))
	}

	// 4. Encrypt
	plaintext := "hello-go-test"
	encInput := EncryptInput{
		KeyId:     keyId,
		Plaintext: []byte(plaintext),
	}
	bodyBytes, _ = json.Marshal(encInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.Encrypt")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var encOutput EncryptOutput
	_ = json.NewDecoder(resp.Body).Decode(&encOutput)
	if len(encOutput.CiphertextBlob) == 0 {
		t.Errorf("expected non-empty ciphertext blob")
	}

	// 5. Decrypt
	decInput := DecryptInput{
		CiphertextBlob: encOutput.CiphertextBlob,
		KeyId:          keyId,
	}
	bodyBytes, _ = json.Marshal(decInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.Decrypt")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var decOutput DecryptOutput
	_ = json.NewDecoder(resp.Body).Decode(&decOutput)
	if string(decOutput.Plaintext) != plaintext {
		t.Errorf("expected decrypted plaintext %s, got %s", plaintext, string(decOutput.Plaintext))
	}

	// 6. Generate Data Key
	genInput := GenerateDataKeyInput{
		KeyId:   keyId,
		KeySpec: "AES_256",
	}
	bodyBytes, _ = json.Marshal(genInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "TrentService.GenerateDataKey")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var genOutput GenerateDataKeyOutput
	_ = json.NewDecoder(resp.Body).Decode(&genOutput)
	if len(genOutput.Plaintext) != 32 {
		t.Errorf("expected 32 byte plaintext key for AES_256, got %d", len(genOutput.Plaintext))
	}
}
