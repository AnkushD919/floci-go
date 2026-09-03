package secretsmanager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecretsManagerLifecycle(t *testing.T) {
	handler := NewHandler()

	name := "test-secret"
	secretString := `{"key":"value"}`
	var secretARN string

	// 1. Create Secret
	createInput := CreateSecretInput{
		Name:         name,
		SecretString: secretString,
		Description:  "My test secret",
	}
	bodyBytes, _ := json.Marshal(createInput)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.CreateSecret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateSecret status 200, got %d", resp.StatusCode)
	}

	var createOutput CreateSecretOutput
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	secretARN = createOutput.ARN
	if secretARN == "" {
		t.Errorf("expected non-empty ARN")
	}

	// 2. Get Secret Value
	getInput := GetSecretValueInput{
		SecretId: secretARN,
	}
	bodyBytes, _ = json.Marshal(getInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var getOutput GetSecretValueOutput
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	if getOutput.SecretString != secretString {
		t.Errorf("expected SecretString %s, got %s", secretString, getOutput.SecretString)
	}

	// 3. Put Secret Value
	newSecretString := `{"key":"new-value"}`
	putInput := PutSecretValueInput{
		SecretId:     name,
		SecretString: newSecretString,
	}
	bodyBytes, _ = json.Marshal(putInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.PutSecretValue")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected PutSecretValue status 200, got %d", resp.StatusCode)
	}

	// 4. Verify new secret value
	bodyBytes, _ = json.Marshal(getInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	if getOutput.SecretString != newSecretString {
		t.Errorf("expected new SecretString %s, got %s", newSecretString, getOutput.SecretString)
	}

	// 5. Delete Secret
	delInput := DeleteSecretInput{
		SecretId:                   secretARN,
		ForceDeleteWithoutRecovery: true,
	}
	bodyBytes, _ = json.Marshal(delInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.DeleteSecret")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteSecret status 200, got %d", resp.StatusCode)
	}

	// 6. Verify Deleted (Get should return 400 ResourceNotFoundException)
	bodyBytes, _ = json.Marshal(getInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected GetSecretValue on deleted secret to return 400, got %d", resp.StatusCode)
	}
}
