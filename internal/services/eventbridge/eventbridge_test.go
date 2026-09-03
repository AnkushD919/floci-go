package eventbridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/floci-io/floci-go/internal/services/lambda"
)

func TestEventBridgeLifecycle(t *testing.T) {
	lambdaH := lambda.NewHandler()
	handler := NewHandler(lambdaH)

	// 1. PutRule
	putRuleInput := map[string]interface{}{
		"Name":         "s3-rule",
		"EventPattern": `{"source":["aws.s3"],"detail-type":["Object Created"]}`,
		"State":        "ENABLED",
	}
	bodyBytes, _ := json.Marshal(putRuleInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSEvents.PutRule")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PutRule status 200, got %d", resp.StatusCode)
	}

	var putRuleOutput map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&putRuleOutput)
	if putRuleOutput["RuleArn"] == "" {
		t.Fatalf("expected rule ARN in output")
	}

	// 2. PutTargets
	putTargetsInput := map[string]interface{}{
		"Rule": "s3-rule",
		"Targets": []Target{
			{
				ID:  "target-1",
				Arn: "arn:aws:lambda:us-east-1:000000000000:function:my-function",
			},
		},
	}
	bodyBytes, _ = json.Marshal(putTargetsInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSEvents.PutTargets")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PutTargets status 200, got %d", resp.StatusCode)
	}

	// 3. PutEvents (Matching)
	putEventsInput := map[string]interface{}{
		"Entries": []map[string]interface{}{
			{
				"Source":     "aws.s3",
				"DetailType": "Object Created",
				"Detail":     `{"bucket":{"name":"test-bucket"}}`,
			},
		},
	}
	bodyBytes, _ = json.Marshal(putEventsInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSEvents.PutEvents")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PutEvents status 200, got %d", resp.StatusCode)
	}

	// 4. Test scheduler rate parsing
	dur, err := parseScheduleRate("rate(5 minutes)")
	if err != nil || dur != 5*time.Minute {
		t.Errorf("expected 5 minutes duration, got %v (error: %v)", dur, err)
	}

	dur, err = parseScheduleRate("rate(1 hour)")
	if err != nil || dur != time.Hour {
		t.Errorf("expected 1 hour duration, got %v (error: %v)", dur, err)
	}
}
