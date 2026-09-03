package lambda

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLambdaLifecycle(t *testing.T) {
	handler := NewHandler()

	// 1. Create Function
	createInput := CreateFunctionInput{
		FunctionName: "test-func",
		Runtime:      "nodejs20.x",
		Handler:      "index.handler",
		Code: FunctionCode{
			ZipFile: []byte("mock-zip-content"),
		},
		Description: "test function",
		Timeout:     5,
		MemorySize:  256,
	}

	bodyBytes, _ := json.Marshal(createInput)
	req := httptest.NewRequest(http.MethodPost, "/2015-03-31/functions", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected CreateFunction status 201, got %d", resp.StatusCode)
	}

	var createOutput FunctionConfiguration
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	if createOutput.FunctionName != "test-func" {
		t.Errorf("expected function name 'test-func', got '%s'", createOutput.FunctionName)
	}
	if createOutput.Timeout != 5 {
		t.Errorf("expected timeout 5, got %d", createOutput.Timeout)
	}

	// 2. Get Function
	req = httptest.NewRequest(http.MethodGet, "/2015-03-31/functions/test-func", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GetFunction status 200, got %d", resp.StatusCode)
	}

	var getOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	config, ok := getOutput["Configuration"].(map[string]interface{})
	if !ok || config["FunctionName"] != "test-func" {
		t.Errorf("expected configuration with FunctionName 'test-func'")
	}

	// 3. List Functions
	req = httptest.NewRequest(http.MethodGet, "/2015-03-31/functions", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ListFunctions status 200, got %d", resp.StatusCode)
	}

	var listOutput ListFunctionsOutput
	_ = json.NewDecoder(resp.Body).Decode(&listOutput)
	if len(listOutput.Functions) != 1 {
		t.Errorf("expected 1 function in list, got %d", len(listOutput.Functions))
	}

	// 4. Update Function Code
	updateInput := FunctionCode{
		ZipFile: []byte("new-mock-zip-content"),
	}
	bodyBytes, _ = json.Marshal(updateInput)
	req = httptest.NewRequest(http.MethodPut, "/2015-03-31/functions/test-func/code", bytes.NewReader(bodyBytes))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected UpdateFunctionCode status 200, got %d", resp.StatusCode)
	}

	// Verify update in snapshot/list
	funcsSnapshot := handler.GetFunctions()
	if len(funcsSnapshot) != 1 || funcsSnapshot[0].CodeSize != int64(len("new-mock-zip-content")) {
		t.Errorf("expected code size %d after update, got %d", len("new-mock-zip-content"), funcsSnapshot[0].CodeSize)
	}

	// 5. Delete Function
	req = httptest.NewRequest(http.MethodDelete, "/2015-03-31/functions/test-func", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected DeleteFunction status 204, got %d", resp.StatusCode)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/2015-03-31/functions/test-func", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected GetFunction on deleted function to return 404, got %d", resp.StatusCode)
	}
}
