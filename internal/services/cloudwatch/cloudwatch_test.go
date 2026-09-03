package cloudwatch

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloudWatchLogsLifecycle(t *testing.T) {
	handler := NewHandler()

	// 1. CreateLogGroup
	createGroupInput := map[string]string{
		"logGroupName": "test-group",
	}
	bodyBytes, _ := json.Marshal(createGroupInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "Logs_20140530.CreateLogGroup")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected CreateLogGroup status 200, got %d", resp.StatusCode)
	}

	// 2. CreateLogStream
	createStreamInput := map[string]string{
		"logGroupName":  "test-group",
		"logStreamName": "test-stream",
	}
	bodyBytes, _ = json.Marshal(createStreamInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "Logs_20140530.CreateLogStream")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected CreateLogStream status 200, got %d", resp.StatusCode)
	}

	// 3. PutLogEvents
	putEventsInput := map[string]interface{}{
		"logGroupName":  "test-group",
		"logStreamName": "test-stream",
		"logEvents": []LogEvent{
			{Timestamp: time.Now().UnixNano() / 1e6, Message: "hello-world-log"},
		},
	}
	bodyBytes, _ = json.Marshal(putEventsInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "Logs_20140530.PutLogEvents")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PutLogEvents status 200, got %d", resp.StatusCode)
	}

	// 4. GetLogEvents
	getEventsInput := map[string]interface{}{
		"logGroupName":  "test-group",
		"logStreamName": "test-stream",
	}
	bodyBytes, _ = json.Marshal(getEventsInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "Logs_20140530.GetLogEvents")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GetLogEvents status 200, got %d", resp.StatusCode)
	}

	var getEventsOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&getEventsOutput)
	events, ok := getEventsOutput["events"].([]interface{})
	if !ok || len(events) != 1 {
		t.Errorf("expected 1 log event, got %d", len(events))
	}
}

func TestCloudWatchMetricsLifecycle(t *testing.T) {
	handler := NewHandler()

	// 1. PutMetricData
	putMetricsInput := map[string]interface{}{
		"Namespace": "AWS/Lambda",
		"MetricData": []MetricDatum{
			{
				MetricName: "Duration",
				Value:      150.0,
				Unit:       "Milliseconds",
				Timestamp:  float64(time.Now().Unix()),
			},
		},
	}
	bodyBytes, _ := json.Marshal(putMetricsInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "GraniteServiceVersion20100801.PutMetricData")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PutMetricData status 200, got %d", resp.StatusCode)
	}

	// 2. ListMetrics
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Amz-Target", "GraniteServiceVersion20100801.ListMetrics")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ListMetrics status 200, got %d", resp.StatusCode)
	}

	var listMetricsOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&listMetricsOutput)
	metrics, ok := listMetricsOutput["Metrics"].([]interface{})
	if !ok || len(metrics) != 1 {
		t.Errorf("expected 1 metric, got %d", len(metrics))
	}

	// 3. GetMetricStatistics
	getStatsInput := map[string]interface{}{
		"Namespace":  "AWS/Lambda",
		"MetricName": "Duration",
		"StartTime":  float64(time.Now().Add(-10 * time.Minute).Unix()),
		"EndTime":    float64(time.Now().Add(10 * time.Minute).Unix()),
		"Period":     60,
		"Statistics": []string{"Average", "Sum"},
	}
	bodyBytes, _ = json.Marshal(getStatsInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "GraniteServiceVersion20100801.GetMetricStatistics")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GetMetricStatistics status 200, got %d", resp.StatusCode)
	}

	var getStatsOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&getStatsOutput)
	dps, ok := getStatsOutput["Datapoints"].([]interface{})
	if !ok || len(dps) != 1 {
		t.Errorf("expected 1 statistic datapoint, got %d", len(dps))
	}
}
