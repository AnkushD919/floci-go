package sqs

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/floci-io/floci-go/internal/awserr"
)

type SQSMessage struct {
	MessageID     string `json:"MessageId" xml:"MessageId"`
	ReceiptHandle string `json:"ReceiptHandle" xml:"ReceiptHandle"`
	Body          string `json:"Body" xml:"Body"`
	MD5OfBody     string `json:"MD5OfBody" xml:"MD5OfBody"`
}

type SQSQueue struct {
	Name       string
	URL        string
	ARN        string
	Attributes map[string]string
	Messages   []SQSMessage
}

type Registry struct {
	mu     sync.RWMutex
	queues map[string]*SQSQueue // URL -> Queue
	arns   map[string]string    // ARN -> URL
}

func NewRegistry() *Registry {
	return &Registry{
		queues: make(map[string]*SQSQueue),
		arns:   make(map[string]string),
	}
}

var GlobalRegistry = NewRegistry()

func (r *Registry) DeliverMessage(queueARN string, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	url, exists := r.arns[queueARN]
	if !exists {
		return fmt.Errorf("queue not found for ARN: %s", queueARN)
	}

	q, exists := r.queues[url]
	if !exists {
		return fmt.Errorf("queue not found: %s", url)
	}

	msgID := generateUUID()
	q.Messages = append(q.Messages, SQSMessage{
		MessageID:     msgID,
		ReceiptHandle: msgID + "-receipt",
		Body:          body,
		MD5OfBody:     md5Hash(body),
	})
	return nil
}

type SQSHandler struct {
	AccountID string
}

func (h *SQSHandler) Name() string {
	return "sqs"
}

func (h *SQSHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "sqs") {
			return true
		}
	}
	host := strings.ToLower(r.Host)
	if strings.Contains(host, "sqs.") {
		return true
	}
	action := r.URL.Query().Get("Action")
	if action == "" && strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}
	return action != "" && strings.HasSuffix(action, "Queue")
}

func NewHandler() *SQSHandler {
	return &SQSHandler{
		AccountID: "000000000000",
	}
}

// Request types & outputs
type CreateQueueJSONInput struct {
	QueueName  string            `json:"QueueName"`
	Attributes map[string]string `json:"Attributes"`
}

type CreateQueueJSONOutput struct {
	QueueUrl string `json:"QueueUrl"`
}

type CreateQueueXMLResult struct {
	QueueUrl string `xml:"QueueUrl"`
}

type CreateQueueXMLResponse struct {
	XMLName   xml.Name             `xml:"CreateQueueResponse"`
	Result    CreateQueueXMLResult `xml:"CreateQueueResult"`
	RequestID string               `xml:"ResponseMetadata>RequestId"`
}

type ListQueuesJSONOutput struct {
	QueueUrls []string `json:"QueueUrls"`
}

type ListQueuesXMLResult struct {
	QueueUrls []string `xml:"QueueUrl"`
}

type ListQueuesXMLResponse struct {
	XMLName   xml.Name            `xml:"ListQueuesResponse"`
	Result    ListQueuesXMLResult `xml:"ListQueuesResult"`
	RequestID string              `xml:"ResponseMetadata>RequestId"`
}

type GetQueueUrlJSONInput struct {
	QueueName string `json:"QueueName"`
}

type GetQueueUrlJSONOutput struct {
	QueueUrl string `json:"QueueUrl"`
}

type GetQueueUrlXMLResult struct {
	QueueUrl string `xml:"QueueUrl"`
}

type GetQueueUrlXMLResponse struct {
	XMLName   xml.Name             `xml:"GetQueueUrlResponse"`
	Result    GetQueueUrlXMLResult `xml:"GetQueueUrlResult"`
	RequestID string               `xml:"ResponseMetadata>RequestId"`
}

type GetQueueAttributesJSONInput struct {
	QueueUrl       string   `json:"QueueUrl"`
	AttributeNames []string `json:"AttributeNames"`
}

type GetQueueAttributesJSONOutput struct {
	Attributes map[string]string `json:"Attributes"`
}

type AttributeEntry struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

type GetQueueAttributesXMLResult struct {
	Attributes []AttributeEntry `xml:"Attribute"`
}

type GetQueueAttributesXMLResponse struct {
	XMLName   xml.Name                    `xml:"GetQueueAttributesResponse"`
	Result    GetQueueAttributesXMLResult `xml:"GetQueueAttributesResult"`
	RequestID string                      `xml:"ResponseMetadata>RequestId"`
}

type SendMessageJSONInput struct {
	QueueUrl    string `json:"QueueUrl"`
	MessageBody string `json:"MessageBody"`
}

type SendMessageJSONOutput struct {
	MessageId       string `json:"MessageId"`
	MD5OfMessageBody string `json:"MD5OfMessageBody"`
}

type SendMessageXMLResult struct {
	MessageId       string `xml:"MessageId"`
	MD5OfMessageBody string `xml:"MD5OfMessageBody"`
}

type SendMessageXMLResponse struct {
	XMLName   xml.Name             `xml:"SendMessageResponse"`
	Result    SendMessageXMLResult `xml:"SendMessageResult"`
	RequestID string               `xml:"ResponseMetadata>RequestId"`
}

type SendMessageBatchJSONEntry struct {
	Id          string `json:"Id"`
	MessageBody string `json:"MessageBody"`
}

type SendMessageBatchJSONInput struct {
	QueueUrl string                      `json:"QueueUrl"`
	Entries  []SendMessageBatchJSONEntry `json:"Entries"`
}

type BatchResultJSONEntry struct {
	Id               string `json:"Id"`
	MessageId        string `json:"MessageId"`
	MD5OfMessageBody string `json:"MD5OfMessageBody"`
}

type SendMessageBatchJSONOutput struct {
	Successful []BatchResultJSONEntry `json:"Successful"`
	Failed     []string               `json:"Failed"`
}

type BatchResultXMLEntry struct {
	Id               string `xml:"Id"`
	MessageId        string `xml:"MessageId"`
	MD5OfMessageBody string `xml:"MD5OfMessageBody"`
}

type SendMessageBatchXMLResult struct {
	Successful []BatchResultXMLEntry `xml:"SendMessageBatchResultEntry"`
}

type SendMessageBatchXMLResponse struct {
	XMLName   xml.Name                  `xml:"SendMessageBatchResponse"`
	Result    SendMessageBatchXMLResult `xml:"SendMessageBatchResult"`
	RequestID string                    `xml:"ResponseMetadata>RequestId"`
}

type ReceiveMessageJSONInput struct {
	QueueUrl            string `json:"QueueUrl"`
	MaxNumberOfMessages int    `json:"MaxNumberOfMessages"`
	WaitTimeSeconds     int    `json:"WaitTimeSeconds"`
}

type ReceiveMessageJSONOutput struct {
	Messages []SQSMessage `json:"Messages"`
}

type ReceiveMessageXMLResult struct {
	Messages []SQSMessage `xml:"Message"`
}

type ReceiveMessageXMLResponse struct {
	XMLName   xml.Name                `xml:"ReceiveMessageResponse"`
	Result    ReceiveMessageXMLResult `xml:"ReceiveMessageResult"`
	RequestID string                  `xml:"ResponseMetadata>RequestId"`
}

type DeleteMessageJSONInput struct {
	QueueUrl      string `json:"QueueUrl"`
	ReceiptHandle string `json:"ReceiptHandle"`
}

type DeleteMessageXMLResponse struct {
	XMLName   xml.Name `xml:"DeleteMessageResponse"`
	RequestID string   `xml:"ResponseMetadata>RequestId"`
}

type SetQueueAttributesJSONInput struct {
	QueueUrl   string            `json:"QueueUrl"`
	Attributes map[string]string `json:"Attributes"`
}

type SetQueueAttributesXMLResponse struct {
	XMLName   xml.Name `xml:"SetQueueAttributesResponse"`
	RequestID string   `xml:"ResponseMetadata>RequestId"`
}

type PurgeQueueJSONInput struct {
	QueueUrl string `json:"QueueUrl"`
}

type PurgeQueueXMLResponse struct {
	XMLName   xml.Name `xml:"PurgeQueueResponse"`
	RequestID string   `xml:"ResponseMetadata>RequestId"`
}

type DeleteQueueJSONInput struct {
	QueueUrl string `json:"QueueUrl"`
}

type DeleteQueueXMLResponse struct {
	XMLName   xml.Name `xml:"DeleteQueueResponse"`
	RequestID string   `xml:"ResponseMetadata>RequestId"`
}

func (h *SQSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	isJSON := strings.Contains(contentType, "application/x-amz-json")

	var action string
	if isJSON {
		target := r.Header.Get("X-Amz-Target")
		parts := strings.Split(target, ".")
		if len(parts) >= 2 {
			action = parts[1]
		}
	} else {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}

	if action == "" {
		// Try to read action from JSON body if target header is not set
		if isJSON {
			// fallback/guess action or read body (we assume target is present for SDK)
		}
	}

	switch action {
	case "CreateQueue":
		h.handleCreateQueue(w, r, isJSON)
	case "ListQueues":
		h.handleListQueues(w, r, isJSON)
	case "GetQueueUrl":
		h.handleGetQueueUrl(w, r, isJSON)
	case "GetQueueAttributes":
		h.handleGetQueueAttributes(w, r, isJSON)
	case "SendMessage":
		h.handleSendMessage(w, r, isJSON)
	case "SendMessageBatch":
		h.handleSendMessageBatch(w, r, isJSON)
	case "ReceiveMessage":
		h.handleReceiveMessage(w, r, isJSON)
	case "DeleteMessage":
		h.handleDeleteMessage(w, r, isJSON)
	case "SetQueueAttributes":
		h.handleSetQueueAttributes(w, r, isJSON)
	case "PurgeQueue":
		h.handlePurgeQueue(w, r, isJSON)
	case "DeleteQueue":
		h.handleDeleteQueue(w, r, isJSON)
	default:
		if isJSON {
			awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
				WriteJSONResponse(w, "AmazonSQS")
		} else {
			awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
				WriteXMLResponse(w, "")
		}
	}
}

func (h *SQSHandler) handleCreateQueue(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qName string
	var attrs map[string]string

	if isJSON {
		var input CreateQueueJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qName = input.QueueName
		attrs = input.Attributes
	} else {
		qName = r.FormValue("QueueName")
		attrs = make(map[string]string)
	}

	if qName == "" {
		h.writeError(w, isJSON, 400, "MissingParameter", "QueueName is required.")
		return
	}

	qURL := fmt.Sprintf("http://%s/000000000000/%s", r.Host, qName)
	qARN := fmt.Sprintf("arn:aws:sqs:us-east-1:%s:%s", h.AccountID, qName)

	if attrs == nil {
		attrs = make(map[string]string)
	}
	attrs["QueueArn"] = qARN

	q := &SQSQueue{
		Name:       qName,
		URL:        qURL,
		ARN:        qARN,
		Attributes: attrs,
		Messages:   make([]SQSMessage, 0),
	}

	GlobalRegistry.queues[qURL] = q
	GlobalRegistry.arns[qARN] = qURL

	if isJSON {
		writeJSON(w, CreateQueueJSONOutput{QueueUrl: qURL})
	} else {
		writeXML(w, CreateQueueXMLResponse{
			Result:    CreateQueueXMLResult{QueueUrl: qURL},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleListQueues(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.RLock()
	defer GlobalRegistry.mu.RUnlock()

	urls := make([]string, 0, len(GlobalRegistry.queues))
	for url := range GlobalRegistry.queues {
		urls = append(urls, url)
	}

	if isJSON {
		writeJSON(w, ListQueuesJSONOutput{QueueUrls: urls})
	} else {
		writeXML(w, ListQueuesXMLResponse{
			Result:    ListQueuesXMLResult{QueueUrls: urls},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleGetQueueUrl(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.RLock()
	defer GlobalRegistry.mu.RUnlock()

	var qName string
	if isJSON {
		var input GetQueueUrlJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qName = input.QueueName
	} else {
		qName = r.FormValue("QueueName")
	}

	var qURL string
	for _, q := range GlobalRegistry.queues {
		if q.Name == qName {
			qURL = q.URL
			break
		}
	}

	if qURL == "" {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	if isJSON {
		writeJSON(w, GetQueueUrlJSONOutput{QueueUrl: qURL})
	} else {
		writeXML(w, GetQueueUrlXMLResponse{
			Result:    GetQueueUrlXMLResult{QueueUrl: qURL},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleGetQueueAttributes(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.RLock()
	defer GlobalRegistry.mu.RUnlock()

	var qURL string
	if isJSON {
		var input GetQueueAttributesJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
	} else {
		qURL = r.FormValue("QueueUrl")
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	if isJSON {
		writeJSON(w, GetQueueAttributesJSONOutput{Attributes: q.Attributes})
	} else {
		entries := make([]AttributeEntry, 0, len(q.Attributes))
		for k, v := range q.Attributes {
			entries = append(entries, AttributeEntry{Name: k, Value: v})
		}
		writeXML(w, GetQueueAttributesXMLResponse{
			Result:    GetQueueAttributesXMLResult{Attributes: entries},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleSendMessage(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	var body string

	if isJSON {
		var input SendMessageJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
		body = input.MessageBody
	} else {
		qURL = r.FormValue("QueueUrl")
		body = r.FormValue("MessageBody")
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	msgID := generateUUID()
	hash := md5Hash(body)

	q.Messages = append(q.Messages, SQSMessage{
		MessageID:     msgID,
		ReceiptHandle: msgID + "-receipt",
		Body:          body,
		MD5OfBody:     hash,
	})

	if isJSON {
		writeJSON(w, SendMessageJSONOutput{
			MessageId:       msgID,
			MD5OfMessageBody: hash,
		})
	} else {
		writeXML(w, SendMessageXMLResponse{
			Result: SendMessageXMLResult{
				MessageId:       msgID,
				MD5OfMessageBody: hash,
			},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleSendMessageBatch(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	var entries []SendMessageBatchJSONEntry

	if isJSON {
		var input SendMessageBatchJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
		entries = input.Entries
	} else {
		qURL = r.FormValue("QueueUrl")
		// Fallback Form-encoded batch extraction
		for i := 1; ; i++ {
			id := r.FormValue(fmt.Sprintf("SendMessageBatchRequestEntry.%d.Id", i))
			if id == "" {
				id = r.FormValue(fmt.Sprintf("Entries.%d.Id", i))
				if id == "" {
					break
				}
			}
			body := r.FormValue(fmt.Sprintf("SendMessageBatchRequestEntry.%d.MessageBody", i))
			if body == "" {
				body = r.FormValue(fmt.Sprintf("Entries.%d.MessageBody", i))
			}
			entries = append(entries, SendMessageBatchJSONEntry{Id: id, MessageBody: body})
		}
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	successfulJSON := make([]BatchResultJSONEntry, 0, len(entries))
	successfulXML := make([]BatchResultXMLEntry, 0, len(entries))

	for _, entry := range entries {
		msgID := generateUUID()
		hash := md5Hash(entry.MessageBody)

		q.Messages = append(q.Messages, SQSMessage{
			MessageID:     msgID,
			ReceiptHandle: msgID + "-receipt",
			Body:          entry.MessageBody,
			MD5OfBody:     hash,
		})

		successfulJSON = append(successfulJSON, BatchResultJSONEntry{
			Id:               entry.Id,
			MessageId:        msgID,
			MD5OfMessageBody: hash,
		})
		successfulXML = append(successfulXML, BatchResultXMLEntry{
			Id:               entry.Id,
			MessageId:        msgID,
			MD5OfMessageBody: hash,
		})
	}

	if isJSON {
		writeJSON(w, SendMessageBatchJSONOutput{
			Successful: successfulJSON,
			Failed:     make([]string, 0),
		})
	} else {
		writeXML(w, SendMessageBatchXMLResponse{
			Result:    SendMessageBatchXMLResult{Successful: successfulXML},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleReceiveMessage(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	maxMessages := 1

	if isJSON {
		var input ReceiveMessageJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
		if input.MaxNumberOfMessages > 0 {
			maxMessages = input.MaxNumberOfMessages
		}
	} else {
		qURL = r.FormValue("QueueUrl")
		if maxStr := r.FormValue("MaxNumberOfMessages"); maxStr != "" {
			if m, err := strconv.Atoi(maxStr); err == nil && m > 0 {
				maxMessages = m
			}
		}
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	// Receive messages (non-destructive read - standard SQS would lock them via VisibilityTimeout)
	// For mock convenience, we return the messages. We do NOT delete them yet.
	limit := maxMessages
	if len(q.Messages) < limit {
		limit = len(q.Messages)
	}

	received := q.Messages[:limit]

	if isJSON {
		writeJSON(w, ReceiveMessageJSONOutput{Messages: received})
	} else {
		writeXML(w, ReceiveMessageXMLResponse{
			Result:    ReceiveMessageXMLResult{Messages: received},
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleDeleteMessage(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	var handle string

	if isJSON {
		var input DeleteMessageJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
		handle = input.ReceiptHandle
	} else {
		qURL = r.FormValue("QueueUrl")
		handle = r.FormValue("ReceiptHandle")
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	// Delete message matching receipt handle
	foundIdx := -1
	for idx, msg := range q.Messages {
		if msg.ReceiptHandle == handle {
			foundIdx = idx
			break
		}
	}

	if foundIdx != -1 {
		q.Messages = append(q.Messages[:foundIdx], q.Messages[foundIdx+1:]...)
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	} else {
		writeXML(w, DeleteMessageXMLResponse{
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleSetQueueAttributes(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	var attrs map[string]string

	if isJSON {
		var input SetQueueAttributesJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
		attrs = input.Attributes
	} else {
		qURL = r.FormValue("QueueUrl")
		attrs = make(map[string]string)
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	for k, v := range attrs {
		q.Attributes[k] = v
	}

	if isJSON {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	} else {
		writeXML(w, SetQueueAttributesXMLResponse{
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handlePurgeQueue(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	if isJSON {
		var input PurgeQueueJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
	} else {
		qURL = r.FormValue("QueueUrl")
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	q.Messages = make([]SQSMessage, 0)

	if isJSON {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	} else {
		writeXML(w, PurgeQueueXMLResponse{
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) handleDeleteQueue(w http.ResponseWriter, r *http.Request, isJSON bool) {
	GlobalRegistry.mu.Lock()
	defer GlobalRegistry.mu.Unlock()

	var qURL string
	if isJSON {
		var input DeleteQueueJSONInput
		_ = json.NewDecoder(r.Body).Decode(&input)
		qURL = input.QueueUrl
	} else {
		qURL = r.FormValue("QueueUrl")
	}

	q, exists := GlobalRegistry.queues[qURL]
	if !exists {
		h.writeError(w, isJSON, 400, "QueueDoesNotExist", "The specified queue does not exist.")
		return
	}

	delete(GlobalRegistry.queues, qURL)
	delete(GlobalRegistry.arns, q.ARN)

	if isJSON {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	} else {
		writeXML(w, DeleteQueueXMLResponse{
			RequestID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func (h *SQSHandler) writeError(w http.ResponseWriter, isJSON bool, status int, code string, msg string) {
	if isJSON {
		awserr.New(status, code, msg).WriteJSONResponse(w, "AmazonSQS")
	} else {
		awserr.New(status, code, msg).WriteXMLResponse(w, "")
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func writeXML(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(data)
}

func md5Hash(text string) string {
	hasher := md5.New()
	hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func generateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

func (r *Registry) GetQueues() []*SQSQueue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*SQSQueue, 0, len(r.queues))
	for _, q := range r.queues {
		qCopy := &SQSQueue{
			Name:       q.Name,
			URL:        q.URL,
			ARN:        q.ARN,
			Attributes: make(map[string]string),
			Messages:   append([]SQSMessage{}, q.Messages...),
		}
		for k, v := range q.Attributes {
			qCopy.Attributes[k] = v
		}
		res = append(res, qCopy)
	}
	return res
}
