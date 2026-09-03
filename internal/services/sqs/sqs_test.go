package sqs

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSQSLifecycle(t *testing.T) {
	handler := NewHandler()

	name := "test-queue"
	var queueUrl string

	// 1. Create Queue (JSON)
	createInput := CreateQueueJSONInput{
		QueueName: name,
	}
	bodyBytes, _ := json.Marshal(createInput)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.CreateQueue")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateQueue status 200, got %d", resp.StatusCode)
	}

	var createOutput CreateQueueJSONOutput
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	queueUrl = createOutput.QueueUrl
	if queueUrl == "" {
		t.Errorf("expected non-empty queue URL")
	}

	// 2. List Queues (JSON)
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.ListQueues")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var listOutput ListQueuesJSONOutput
	_ = json.NewDecoder(resp.Body).Decode(&listOutput)
	if len(listOutput.QueueUrls) == 0 {
		t.Errorf("expected at least 1 queue URL")
	}

	// 3. Send Message (JSON)
	sendInput := SendMessageJSONInput{
		QueueUrl:    queueUrl,
		MessageBody: "test-body",
	}
	bodyBytes, _ = json.Marshal(sendInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.SendMessage")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var sendOutput SendMessageJSONOutput
	_ = json.NewDecoder(resp.Body).Decode(&sendOutput)
	if sendOutput.MessageId == "" {
		t.Errorf("expected non-empty message ID")
	}

	// 4. Receive Message (JSON)
	recvInput := ReceiveMessageJSONInput{
		QueueUrl:            queueUrl,
		MaxNumberOfMessages: 10,
	}
	bodyBytes, _ = json.Marshal(recvInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.ReceiveMessage")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var recvOutput ReceiveMessageJSONOutput
	_ = json.NewDecoder(resp.Body).Decode(&recvOutput)
	if len(recvOutput.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(recvOutput.Messages))
	}
	if recvOutput.Messages[0].Body != "test-body" {
		t.Errorf("expected body test-body, got %s", recvOutput.Messages[0].Body)
	}

	// 5. Delete Message (JSON)
	delInput := DeleteMessageJSONInput{
		QueueUrl:      queueUrl,
		ReceiptHandle: recvOutput.Messages[0].ReceiptHandle,
	}
	bodyBytes, _ = json.Marshal(delInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.DeleteMessage")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteMessage status 200, got %d", resp.StatusCode)
	}

	// 6. Delete Queue (JSON)
	delQueueInput := DeleteQueueJSONInput{
		QueueUrl: queueUrl,
	}
	bodyBytes, _ = json.Marshal(delQueueInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS.DeleteQueue")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteQueue status 200, got %d", resp.StatusCode)
	}
}

func TestSQSFormProtocol(t *testing.T) {
	handler := NewHandler()

	// Create Queue (Form)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = map[string][]string{
		"Action":    {"CreateQueue"},
		"QueueName": {"form-queue"},
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateQueue status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<QueueUrl>") {
		t.Errorf("expected QueueUrl tag in XML, got %s", bodyStr)
	}
}
