package sns

import (
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/floci-io/floci-go/internal/awserr"
	"github.com/floci-io/floci-go/internal/services/sqs"
)

const (
	snsNamespace = "http://sns.amazonaws.com/doc/2010-03-31/"
)

type SNSTopic struct {
	ARN        string
	Name       string
	Attributes map[string]string
}

type SNSSubscription struct {
	ARN        string
	TopicARN   string
	Protocol   string
	Endpoint   string
	Attributes map[string]string
}

type SNSHandler struct {
	mu            sync.RWMutex
	topics        map[string]*SNSTopic
	subscriptions map[string]*SNSSubscription
	AccountID     string
}

func (h *SNSHandler) Name() string {
	return "sns"
}

func (h *SNSHandler) Matches(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	if strings.Contains(host, "sns.") {
		return true
	}
	action := r.URL.Query().Get("Action")
	if action == "" && strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}
	return action != "" && (strings.HasSuffix(action, "Topic") || strings.HasSuffix(action, "Subscription"))
}

func NewHandler() *SNSHandler {
	return &SNSHandler{
		topics:        make(map[string]*SNSTopic),
		subscriptions: make(map[string]*SNSSubscription),
		AccountID:     "000000000000",
	}
}

// XML responses structures
type ResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

type CreateTopicResult struct {
	TopicArn string `xml:"TopicArn"`
}

type CreateTopicResponse struct {
	XMLName          xml.Name          `xml:"CreateTopicResponse"`
	Xmlns            string            `xml:"xmlns,attr"`
	Result           CreateTopicResult `xml:"CreateTopicResult"`
	ResponseMetadata ResponseMetadata  `xml:"ResponseMetadata"`
}

type TopicMember struct {
	TopicArn string `xml:"TopicArn"`
}

type ListTopicsResult struct {
	Topics []TopicMember `xml:"Topics>member"`
}

type ListTopicsResponse struct {
	XMLName          xml.Name         `xml:"ListTopicsResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	Result           ListTopicsResult `xml:"ListTopicsResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type AttributeEntry struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

type GetTopicAttributesResult struct {
	Attributes []AttributeEntry `xml:"Attributes>entry"`
}

type GetTopicAttributesResponse struct {
	XMLName          xml.Name                 `xml:"GetTopicAttributesResponse"`
	Xmlns            string                   `xml:"xmlns,attr"`
	Result           GetTopicAttributesResult `xml:"GetTopicAttributesResult"`
	ResponseMetadata ResponseMetadata         `xml:"ResponseMetadata"`
}

type SubscribeResult struct {
	SubscriptionArn string `xml:"SubscriptionArn"`
}

type SubscribeResponse struct {
	XMLName          xml.Name        `xml:"SubscribeResponse"`
	Xmlns            string          `xml:"xmlns,attr"`
	Result           SubscribeResult `xml:"SubscribeResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type SubscriptionMember struct {
	SubscriptionArn string `xml:"SubscriptionArn"`
	Owner           string `xml:"Owner"`
	Protocol        string `xml:"Protocol"`
	Endpoint        string `xml:"Endpoint"`
	TopicArn        string `xml:"TopicArn"`
}

type ListSubscriptionsResult struct {
	Subscriptions []SubscriptionMember `xml:"Subscriptions>member"`
}

type ListSubscriptionsResponse struct {
	XMLName          xml.Name                `xml:"ListSubscriptionsResponse"`
	Xmlns            string                  `xml:"xmlns,attr"`
	Result           ListSubscriptionsResult `xml:"ListSubscriptionsResult"`
	ResponseMetadata ResponseMetadata        `xml:"ResponseMetadata"`
}

type PublishResult struct {
	MessageId string `xml:"MessageId"`
}

type PublishResponse struct {
	XMLName          xml.Name         `xml:"PublishResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	Result           PublishResult    `xml:"PublishResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type GetSubscriptionAttributesResult struct {
	Attributes []AttributeEntry `xml:"Attributes>entry"`
}

type GetSubscriptionAttributesResponse struct {
	XMLName          xml.Name                        `xml:"GetSubscriptionAttributesResponse"`
	Xmlns            string                          `xml:"xmlns,attr"`
	Result           GetSubscriptionAttributesResult `xml:"GetSubscriptionAttributesResult"`
	ResponseMetadata ResponseMetadata                `xml:"ResponseMetadata"`
}

type UnsubscribeResponse struct {
	XMLName          xml.Name         `xml:"UnsubscribeResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type DeleteTopicResponse struct {
	XMLName          xml.Name         `xml:"DeleteTopicResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

func (h *SNSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	action := r.FormValue("Action")

	switch action {
	case "CreateTopic":
		h.handleCreateTopic(w, r)
	case "ListTopics":
		h.handleListTopics(w, r)
	case "GetTopicAttributes":
		h.handleGetTopicAttributes(w, r)
	case "Subscribe":
		h.handleSubscribe(w, r)
	case "ListSubscriptions":
		h.handleListSubscriptions(w, r)
	case "Publish":
		h.handlePublish(w, r)
	case "GetSubscriptionAttributes":
		h.handleGetSubscriptionAttributes(w, r)
	case "Unsubscribe":
		h.handleUnsubscribe(w, r)
	case "DeleteTopic":
		h.handleDeleteTopic(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteXMLResponse(w, snsNamespace)
	}
}

func (h *SNSHandler) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	name := r.FormValue("Name")
	if name == "" {
		awserr.New(400, "MissingParameter", "Topic Name is required.").WriteXMLResponse(w, snsNamespace)
		return
	}

	arn := fmt.Sprintf("arn:aws:sns:us-east-1:%s:%s", h.AccountID, name)

	h.topics[arn] = &SNSTopic{
		ARN:  arn,
		Name: name,
		Attributes: map[string]string{
			"TopicArn":     arn,
			"DisplayName":  name,
			"Owner":        h.AccountID,
			"Policy":       "{}",
			"SubscriptionsPending": "0",
			"SubscriptionsConfirmed": "0",
			"SubscriptionsDeleted": "0",
		},
	}

	writeXML(w, CreateTopicResponse{
		Xmlns: snsNamespace,
		Result: CreateTopicResult{TopicArn: arn},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleListTopics(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	topicsList := make([]TopicMember, 0, len(h.topics))
	for arn := range h.topics {
		topicsList = append(topicsList, TopicMember{TopicArn: arn})
	}

	writeXML(w, ListTopicsResponse{
		Xmlns:  snsNamespace,
		Result: ListTopicsResult{Topics: topicsList},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleGetTopicAttributes(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	arn := r.FormValue("TopicArn")
	topic, exists := h.topics[arn]
	if !exists {
		awserr.New(404, "NotFound", "Topic not found.").WriteXMLResponse(w, snsNamespace)
		return
	}

	entries := make([]AttributeEntry, 0, len(topic.Attributes))
	for k, v := range topic.Attributes {
		entries = append(entries, AttributeEntry{Key: k, Value: v})
	}

	writeXML(w, GetTopicAttributesResponse{
		Xmlns:  snsNamespace,
		Result: GetTopicAttributesResult{Attributes: entries},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	topicArn := r.FormValue("TopicArn")
	protocol := r.FormValue("Protocol")
	endpoint := r.FormValue("Endpoint")

	if topicArn == "" || protocol == "" || endpoint == "" {
		awserr.New(400, "MissingParameter", "TopicArn, Protocol, and Endpoint are required.").
			WriteXMLResponse(w, snsNamespace)
		return
	}

	subArn := fmt.Sprintf("%s:%s", topicArn, generateUUID())

	h.subscriptions[subArn] = &SNSSubscription{
		ARN:      subArn,
		TopicARN: topicArn,
		Protocol: protocol,
		Endpoint: endpoint,
		Attributes: map[string]string{
			"SubscriptionArn": subArn,
			"TopicArn":        topicArn,
			"Endpoint":        endpoint,
			"Protocol":        protocol,
			"Owner":           h.AccountID,
		},
	}

	writeXML(w, SubscribeResponse{
		Xmlns: snsNamespace,
		Result: SubscribeResult{SubscriptionArn: subArn},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	subsList := make([]SubscriptionMember, 0, len(h.subscriptions))
	for _, sub := range h.subscriptions {
		subsList = append(subsList, SubscriptionMember{
			SubscriptionArn: sub.ARN,
			Owner:           h.AccountID,
			Protocol:        sub.Protocol,
			Endpoint:        sub.Endpoint,
			TopicArn:        sub.TopicARN,
		})
	}

	writeXML(w, ListSubscriptionsResponse{
		Xmlns:  snsNamespace,
		Result: ListSubscriptionsResult{Subscriptions: subsList},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handlePublish(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	topicArn := r.FormValue("TopicArn")
	message := r.FormValue("Message")

	if topicArn == "" || message == "" {
		awserr.New(400, "MissingParameter", "TopicArn and Message are required.").
			WriteXMLResponse(w, snsNamespace)
		return
	}

	msgID := generateUUID()

	// Deliver to SQS subscribers
	for _, sub := range h.subscriptions {
		if sub.TopicARN == topicArn && strings.ToLower(sub.Protocol) == "sqs" {
			// Deliver message directly to SQS Registry
			_ = sqs.GlobalRegistry.DeliverMessage(sub.Endpoint, message)
		}
	}

	writeXML(w, PublishResponse{
		Xmlns:  snsNamespace,
		Result: PublishResult{MessageId: msgID},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleGetSubscriptionAttributes(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	arn := r.FormValue("SubscriptionArn")
	sub, exists := h.subscriptions[arn]
	if !exists {
		awserr.New(404, "NotFound", "Subscription not found.").WriteXMLResponse(w, snsNamespace)
		return
	}

	entries := make([]AttributeEntry, 0, len(sub.Attributes))
	for k, v := range sub.Attributes {
		entries = append(entries, AttributeEntry{Key: k, Value: v})
	}

	writeXML(w, GetSubscriptionAttributesResponse{
		Xmlns:  snsNamespace,
		Result: GetSubscriptionAttributesResult{Attributes: entries},
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	arn := r.FormValue("SubscriptionArn")
	if _, exists := h.subscriptions[arn]; !exists {
		awserr.New(404, "NotFound", "Subscription not found.").WriteXMLResponse(w, snsNamespace)
		return
	}

	delete(h.subscriptions, arn)

	writeXML(w, UnsubscribeResponse{
		Xmlns:            snsNamespace,
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func (h *SNSHandler) handleDeleteTopic(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	arn := r.FormValue("TopicArn")
	if _, exists := h.topics[arn]; !exists {
		awserr.New(404, "NotFound", "Topic not found.").WriteXMLResponse(w, snsNamespace)
		return
	}

	delete(h.topics, arn)

	writeXML(w, DeleteTopicResponse{
		Xmlns:            snsNamespace,
		ResponseMetadata: ResponseMetadata{RequestID: "00000000-0000-0000-0000-000000000000"},
	})
}

func writeXML(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(data)
}

func generateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:])
}

func (h *SNSHandler) GetTopics() []SNSTopic {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]SNSTopic, 0, len(h.topics))
	for _, t := range h.topics {
		res = append(res, *t)
	}
	return res
}

func (h *SNSHandler) GetSubscriptions() []SNSSubscription {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]SNSSubscription, 0, len(h.subscriptions))
	for _, s := range h.subscriptions {
		res = append(res, *s)
	}
	return res
}
