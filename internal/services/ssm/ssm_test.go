package ssm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSMParameterLifecycle(t *testing.T) {
	handler := NewHandler()

	name := "/test/param"
	value := "secret-value"

	// 1. Put Parameter
	putInput := PutParameterInput{
		Name:      name,
		Value:     value,
		Type:      "String",
		Overwrite: true,
	}
	bodyBytes, _ := json.Marshal(putInput)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.PutParameter")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected PutParameter status 200, got %d", resp.StatusCode)
	}

	var putOutput PutParameterOutput
	_ = json.NewDecoder(resp.Body).Decode(&putOutput)
	if putOutput.Version != 1 {
		t.Errorf("expected version 1, got %d", putOutput.Version)
	}

	// 2. Put Parameter (fail without overwrite)
	putInput.Overwrite = false
	bodyBytes, _ = json.Marshal(putInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.PutParameter")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected PutParameter overwrite=false to fail with 400, got %d", resp.StatusCode)
	}

	// 3. Get Parameter
	getInput := GetParameterInput{
		Name: name,
	}
	bodyBytes, _ = json.Marshal(getInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected GetParameter status 200, got %d", resp.StatusCode)
	}

	var getOutput GetParameterOutput
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	if getOutput.Parameter.Value != value {
		t.Errorf("expected value %s, got %s", value, getOutput.Parameter.Value)
	}

	// 4. Describe Parameters
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Amz-Target", "AmazonSSM.DescribeParameters")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var descOutput DescribeParametersOutput
	_ = json.NewDecoder(resp.Body).Decode(&descOutput)
	if len(descOutput.Parameters) != 1 {
		t.Errorf("expected 1 parameter in describe list, got %d", len(descOutput.Parameters))
	}

	// 5. Get Parameters By Path
	pathInput := GetParametersByPathInput{
		Path:      "/test",
		Recursive: true,
	}
	bodyBytes, _ = json.Marshal(pathInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParametersByPath")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var pathOutput GetParametersByPathOutput
	_ = json.NewDecoder(resp.Body).Decode(&pathOutput)
	if len(pathOutput.Parameters) != 1 {
		t.Errorf("expected 1 parameter in path match, got %d", len(pathOutput.Parameters))
	}

	// 6. Delete Parameter
	delInput := DeleteParameterInput{
		Name: name,
	}
	bodyBytes, _ = json.Marshal(delInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.DeleteParameter")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteParameter status 200, got %d", resp.StatusCode)
	}

	// 7. Verify Deleted
	getInput = GetParameterInput{
		Name: name,
	}
	bodyBytes, _ = json.Marshal(getInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected GetParameter on deleted param to return 400, got %d", resp.StatusCode)
	}
}
