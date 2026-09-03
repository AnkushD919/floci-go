package eventbridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
	"github.com/floci-io/floci-go/internal/services/lambda"
	"github.com/floci-io/floci-go/internal/services/sqs"
)

type Target struct {
	ID   string `json:"Id"`
	Arn  string `json:"Arn"`
	Input string `json:"Input,omitempty"`
}

type Rule struct {
	Name               string   `json:"Name"`
	Arn                string   `json:"Arn"`
	EventPattern       string   `json:"EventPattern,omitempty"`
	ScheduleExpression string   `json:"ScheduleExpression,omitempty"`
	State              string   `json:"State"` // ENABLED or DISABLED
	Targets            []Target `json:"Targets"`
}

type ScheduleJob struct {
	RuleName string
	StopChan chan struct{}
}

type EventBridgeHandler struct {
	mu            sync.RWMutex
	rules         map[string]*Rule
	lambdaHandler *lambda.LambdaHandler
	scheduleJobs  map[string]*ScheduleJob
	AccountID     string
}

func NewHandler(lambdaHandler *lambda.LambdaHandler) *EventBridgeHandler {
	return &EventBridgeHandler{
		rules:         make(map[string]*Rule),
		lambdaHandler: lambdaHandler,
		scheduleJobs:  make(map[string]*ScheduleJob),
		AccountID:     "000000000000",
	}
}

func (h *EventBridgeHandler) Name() string {
	return "eventbridge"
}

func (h *EventBridgeHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		return len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "events")
	}
	return false
}

func (h *EventBridgeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, "AWSEvents")
		return
	}

	action := parts[1]

	switch action {
	case "PutRule":
		h.handlePutRule(w, r)
	case "PutTargets":
		h.handlePutTargets(w, r)
	case "PutEvents":
		h.handlePutEvents(w, r)
	case "DeleteRule":
		h.handleDeleteRule(w, r)
	case "RemoveTargets":
		h.handleRemoveTargets(w, r)
	case "DescribeRule":
		h.handleDescribeRule(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action %s not supported", action)).
			WriteJSONResponse(w, "AWSEvents")
	}
}

func (h *EventBridgeHandler) handlePutRule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name               string `json:"Name"`
		EventPattern       string `json:"EventPattern"`
		ScheduleExpression string `json:"ScheduleExpression"`
		State              string `json:"State"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	rule, exists := h.rules[input.Name]
	if !exists {
		rule = &Rule{
			Name:    input.Name,
			Arn:     fmt.Sprintf("arn:aws:events:us-east-1:%s:rule/%s", h.AccountID, input.Name),
			Targets: make([]Target, 0),
		}
		h.rules[input.Name] = rule
	}

	rule.EventPattern = input.EventPattern
	rule.ScheduleExpression = input.ScheduleExpression
	rule.State = input.State
	if rule.State == "" {
		rule.State = "ENABLED"
	}
	h.mu.Unlock()

	// If rule has schedule expression, start background scheduler job
	if rule.ScheduleExpression != "" && rule.State == "ENABLED" {
		h.startScheduler(rule.Name, rule.ScheduleExpression)
	} else {
		h.stopScheduler(rule.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"RuleArn": rule.Arn})
}

func (h *EventBridgeHandler) handlePutTargets(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Rule    string   `json:"Rule"`
		Targets []Target `json:"Targets"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	rule, exists := h.rules[input.Rule]
	if !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", "Rule not found").WriteJSONResponse(w, "AWSEvents")
		return
	}

	// Update or add targets
	for _, newTarget := range input.Targets {
		found := false
		for i, existing := range rule.Targets {
			if existing.ID == newTarget.ID {
				rule.Targets[i] = newTarget
				found = true
				break
			}
		}
		if !found {
			rule.Targets = append(rule.Targets, newTarget)
		}
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"FailedEntryCount": 0,
		"FailedEntries":    []interface{}{},
	})
}

func (h *EventBridgeHandler) handlePutEvents(w http.ResponseWriter, r *http.Request) {
	type EventEntry struct {
		Source       string `json:"Source"`
		DetailType   string `json:"DetailType"`
		Detail       string `json:"Detail"`
		EventBusName string `json:"EventBusName"`
	}
	var input struct {
		Entries []EventEntry `json:"Entries"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	entriesOutput := make([]map[string]string, 0, len(input.Entries))

	for _, entry := range input.Entries {
		eventId := fmt.Sprintf("event-%d", time.Now().UnixNano())
		entriesOutput = append(entriesOutput, map[string]string{"EventId": eventId})

		// Route event to matching rules
		for _, rule := range h.rules {
			if rule.State != "ENABLED" {
				continue
			}
			var detailMap map[string]interface{}
			_ = json.Unmarshal([]byte(entry.Detail), &detailMap)

			if matchPattern(entry.Source, entry.DetailType, detailMap, rule.EventPattern) {
				h.triggerTargets(rule.Targets, entry.Detail)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"FailedEntryCount": 0,
		"Entries":          entriesOutput,
	})
}

func (h *EventBridgeHandler) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	if _, exists := h.rules[input.Name]; !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", "Rule not found").WriteJSONResponse(w, "AWSEvents")
		return
	}
	delete(h.rules, input.Name)
	h.mu.Unlock()

	h.stopScheduler(input.Name)

	w.WriteHeader(http.StatusOK)
}

func (h *EventBridgeHandler) handleRemoveTargets(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Rule string   `json:"Rule"`
		Ids  []string `json:"Ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	rule, exists := h.rules[input.Rule]
	if !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", "Rule not found").WriteJSONResponse(w, "AWSEvents")
		return
	}

	newTargets := make([]Target, 0, len(rule.Targets))
	for _, target := range rule.Targets {
		keep := true
		for _, id := range input.Ids {
			if target.ID == id {
				keep = false
				break
			}
		}
		if keep {
			newTargets = append(newTargets, target)
		}
	}
	rule.Targets = newTargets
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"FailedEntryCount": 0,
		"FailedEntries":    []interface{}{},
	})
}

func (h *EventBridgeHandler) handleDescribeRule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"Name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	rule, exists := h.rules[input.Name]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Rule not found").WriteJSONResponse(w, "AWSEvents")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rule)
}

func (h *EventBridgeHandler) triggerTargets(targets []Target, detailJSON string) {
	for _, target := range targets {
		payload := detailJSON
		if target.Input != "" {
			payload = target.Input
		}

		if strings.Contains(target.Arn, "arn:aws:lambda") {
			parts := strings.Split(target.Arn, ":")
			funcName := parts[len(parts)-1]

			go func() {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/2015-03-31/functions/%s/invocations", funcName), strings.NewReader(payload))
				h.lambdaHandler.ServeHTTP(rec, req)
			}()
		} else if strings.Contains(target.Arn, "arn:aws:sqs") {
			go func(arn, msg string) {
				_ = sqs.GlobalRegistry.DeliverMessage(arn, msg)
			}(target.Arn, payload)
		}
	}
}

func (h *EventBridgeHandler) triggerScheduledRule(ruleName string) {
	h.mu.RLock()
	rule, exists := h.rules[ruleName]
	h.mu.RUnlock()

	if !exists || rule.State != "ENABLED" {
		return
	}

	mockDetail := fmt.Sprintf(`{
		"version": "0",
		"id": "scheduled-event-%d",
		"detail-type": "Scheduled Event",
		"source": "aws.events",
		"account": "%s",
		"time": "%s",
		"region": "us-east-1",
		"resources": ["%s"],
		"detail": {}
	}`, time.Now().UnixNano(), h.AccountID, time.Now().Format(time.RFC3339), rule.Arn)

	h.triggerTargets(rule.Targets, mockDetail)
}

func (h *EventBridgeHandler) startScheduler(ruleName, expr string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if job, exists := h.scheduleJobs[ruleName]; exists {
		close(job.StopChan)
		delete(h.scheduleJobs, ruleName)
	}

	dur, err := parseScheduleRate(expr)
	if err != nil {
		log.Printf("[EventBridge] Unsupported schedule rate expression %s: %v", expr, err)
		return
	}

	stopChan := make(chan struct{})
	h.scheduleJobs[ruleName] = &ScheduleJob{
		RuleName: ruleName,
		StopChan: stopChan,
	}

	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.triggerScheduledRule(ruleName)
			case <-stopChan:
				return
			}
		}
	}()
}

func (h *EventBridgeHandler) stopScheduler(ruleName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if job, exists := h.scheduleJobs[ruleName]; exists {
		close(job.StopChan)
		delete(h.scheduleJobs, ruleName)
	}
}

func matchPattern(eventSource, eventDetailType string, detailMap map[string]interface{}, patternJSON string) bool {
	if patternJSON == "" {
		return true
	}
	var pattern map[string]interface{}
	if err := json.Unmarshal([]byte(patternJSON), &pattern); err != nil {
		return false
	}

	for k, v := range pattern {
		switch k {
		case "source":
			allowedSources, ok := toStringSlice(v)
			if !ok || !containsString(allowedSources, eventSource) {
				return false
			}
		case "detail-type":
			allowedTypes, ok := toStringSlice(v)
			if !ok || !containsString(allowedTypes, eventDetailType) {
				return false
			}
		case "detail":
			patternDetail, ok := v.(map[string]interface{})
			if ok {
				for dk, dv := range patternDetail {
					val, exists := detailMap[dk]
					if !exists {
						return false
					}
					allowedVals, ok := toStringSlice(dv)
					if ok {
						if !containsString(allowedVals, fmt.Sprintf("%v", val)) {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func toStringSlice(val interface{}) ([]string, bool) {
	arr, ok := val.([]interface{})
	if !ok {
		str, ok := val.(string)
		if ok {
			return []string{str}, true
		}
		return nil, false
	}
	res := make([]string, 0, len(arr))
	for _, item := range arr {
		if str, ok := item.(string); ok {
			res = append(res, str)
		}
	}
	return res, true
}

func containsString(arr []string, val string) bool {
	for _, item := range arr {
		if item == val {
			return true
		}
	}
	return false
}

func parseScheduleRate(expr string) (time.Duration, error) {
	if !strings.HasPrefix(expr, "rate(") || !strings.HasSuffix(expr, ")") {
		return 0, fmt.Errorf("invalid format")
	}
	inner := expr[5 : len(expr)-1]
	parts := strings.Fields(inner)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid rate expression parts")
	}
	var count int
	if _, err := fmt.Sscanf(parts[0], "%d", &count); err != nil {
		return 0, err
	}
	unit := strings.ToLower(parts[1])
	switch {
	case strings.HasPrefix(unit, "minute"):
		return time.Duration(count) * time.Minute, nil
	case strings.HasPrefix(unit, "hour"):
		return time.Duration(count) * time.Hour, nil
	case strings.HasPrefix(unit, "day"):
		return time.Duration(count) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported rate unit: %s", unit)
	}
}

func (h *EventBridgeHandler) GetRules() []*Rule {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*Rule, 0, len(h.rules))
	for _, rule := range h.rules {
		list = append(list, rule)
	}
	return list
}
