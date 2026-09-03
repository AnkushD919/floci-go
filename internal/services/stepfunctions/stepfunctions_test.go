package stepfunctions

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/floci-io/floci-go/internal/services/lambda"
)

func TestStepFunctionsLifecycle(t *testing.T) {
	lambdaH := lambda.NewHandler()
	handler := NewHandler(lambdaH)

	// ASL State Machine Definition: Pass state -> Succeed
	asl := `{
		"StartAt": "HelloState",
		"States": {
			"HelloState": {
				"Type": "Pass",
				"Result": {"hello": "world"},
				"Next": "FinalState"
			},
			"FinalState": {
				"Type": "Succeed"
			}
		}
	}`

	// 1. CreateStateMachine
	createInput := map[string]string{
		"name":       "test-sm",
		"definition": asl,
		"roleArn":    "arn:aws:iam::000000000000:role/service-role",
	}
	bodyBytes, _ := json.Marshal(createInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSStepFunctions.CreateStateMachine")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected CreateStateMachine status 200, got %d", resp.StatusCode)
	}

	var createOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	smArn, _ := createOutput["stateMachineArn"].(string)
	if smArn == "" {
		t.Fatalf("expected stateMachineArn in output")
	}

	// 2. DescribeStateMachine
	descInput := map[string]string{
		"stateMachineArn": smArn,
	}
	bodyBytes, _ = json.Marshal(descInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSStepFunctions.DescribeStateMachine")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected DescribeStateMachine status 200, got %d", resp.StatusCode)
	}

	// 3. StartExecution
	startInput := map[string]string{
		"stateMachineArn": smArn,
		"input":           `{"foo": "bar"}`,
	}
	bodyBytes, _ = json.Marshal(startInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSStepFunctions.StartExecution")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected StartExecution status 200, got %d", resp.StatusCode)
	}

	var startOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&startOutput)
	execArn, _ := startOutput["executionArn"].(string)
	if execArn == "" {
		t.Fatalf("expected executionArn in output")
	}

	// Wait a moment for async execution to complete
	time.Sleep(100 * time.Millisecond)

	// 4. DescribeExecution
	descExecInput := map[string]string{
		"executionArn": execArn,
	}
	bodyBytes, _ = json.Marshal(descExecInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSStepFunctions.DescribeExecution")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected DescribeExecution status 200, got %d", resp.StatusCode)
	}

	var descExecOutput Execution
	_ = json.NewDecoder(resp.Body).Decode(&descExecOutput)
	if descExecOutput.Status != "SUCCEEDED" {
		t.Errorf("expected execution status SUCCEEDED, got %s", descExecOutput.Status)
	}

	var output map[string]string
	_ = json.Unmarshal([]byte(descExecOutput.Output), &output)
	if output["hello"] != "world" {
		t.Errorf("expected output to contain hello:world, got %+v", output)
	}
}
