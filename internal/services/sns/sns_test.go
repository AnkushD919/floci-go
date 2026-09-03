package sns

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/floci-io/floci-go/internal/services/sqs"
)

func TestSNSLifecycleAndSQSFanout(t *testing.T) {
	snsHandler := NewHandler()
	sqsHandler := sqs.NewHandler()

	topicName := "test-topic"
	queueName := "test-sns-target"

	// 1. Create Topic
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action": []string{"CreateTopic"},
		"Name":   []string{topicName},
	}
	w := httptest.NewRecorder()
	snsHandler.ServeHTTP(w, req)

	var createTopicResp CreateTopicResponse
	_ = xml.NewDecoder(w.Body).Decode(&createTopicResp)
	topicArn := createTopicResp.Result.TopicArn
	if topicArn == "" {
		t.Fatalf("expected non-empty topic ARN")
	}

	// 2. Create SQS Queue to subscribe to
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":    []string{"CreateQueue"},
		"QueueName": []string{queueName},
	}
	w = httptest.NewRecorder()
	sqsHandler.ServeHTTP(w, req)

	var createQueueResp sqs.CreateQueueXMLResponse
	_ = xml.NewDecoder(w.Body).Decode(&createQueueResp)
	queueUrl := createQueueResp.Result.QueueUrl
	if queueUrl == "" {
		t.Fatalf("expected non-empty queue URL")
	}

	// Get Queue Attributes to get ARN
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":   []string{"GetQueueAttributes"},
		"QueueUrl": []string{queueUrl},
	}
	w = httptest.NewRecorder()
	sqsHandler.ServeHTTP(w, req)

	var getQueueAttrsResp sqs.GetQueueAttributesXMLResponse
	_ = xml.NewDecoder(w.Body).Decode(&getQueueAttrsResp)
	var queueArn string
	for _, attr := range getQueueAttrsResp.Result.Attributes {
		if attr.Name == "QueueArn" {
			queueArn = attr.Value
		}
	}
	if queueArn == "" {
		t.Fatalf("expected non-empty queue ARN")
	}

	// 3. Subscribe Queue to Topic
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":   []string{"Subscribe"},
		"TopicArn": []string{topicArn},
		"Protocol": []string{"sqs"},
		"Endpoint": []string{queueArn},
	}
	w = httptest.NewRecorder()
	snsHandler.ServeHTTP(w, req)

	var subscribeResp SubscribeResponse
	_ = xml.NewDecoder(w.Body).Decode(&subscribeResp)
	subArn := subscribeResp.Result.SubscriptionArn
	if subArn == "" {
		t.Fatalf("expected non-empty subscription ARN")
	}

	// 4. Publish Message to Topic
	msgBody := `{"test":"sns-to-sqs"}`
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":   []string{"Publish"},
		"TopicArn": []string{topicArn},
		"Message":  []string{msgBody},
	}
	w = httptest.NewRecorder()
	snsHandler.ServeHTTP(w, req)

	var publishResp PublishResponse
	_ = xml.NewDecoder(w.Body).Decode(&publishResp)
	if publishResp.Result.MessageId == "" {
		t.Errorf("expected non-empty message ID")
	}

	// 5. Verify Message Delivered to SQS Queue
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":   []string{"ReceiveMessage"},
		"QueueUrl": []string{queueUrl},
	}
	w = httptest.NewRecorder()
	sqsHandler.ServeHTTP(w, req)

	var receiveResp sqs.ReceiveMessageXMLResponse
	_ = xml.NewDecoder(w.Body).Decode(&receiveResp)
	if len(receiveResp.Result.Messages) != 1 {
		t.Errorf("expected 1 delivered message in SQS, got %d", len(receiveResp.Result.Messages))
	} else if receiveResp.Result.Messages[0].Body != msgBody {
		t.Errorf("expected delivered body %s, got %s", msgBody, receiveResp.Result.Messages[0].Body)
	}

	// 6. Cleanup (Unsubscribe & Delete Topic)
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":          []string{"Unsubscribe"},
		"SubscriptionArn": []string{subArn},
	}
	w = httptest.NewRecorder()
	snsHandler.ServeHTTP(w, req)

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.PostForm = url.Values{
		"Action":   []string{"DeleteTopic"},
		"TopicArn": []string{topicArn},
	}
	w = httptest.NewRecorder()
	snsHandler.ServeHTTP(w, req)
}
