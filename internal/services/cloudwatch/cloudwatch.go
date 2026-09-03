package cloudwatch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	logsTargetPrefix    = "Logs_20140530"
	metricsTargetPrefix = "GraniteServiceVersion20100801"
)

type LogEvent struct {
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

type LogStream struct {
	Name   string     `json:"logStreamName"`
	Arn    string     `json:"arn"`
	Events []LogEvent `json:"-"`
}

type LogGroup struct {
	Name    string                `json:"logGroupName"`
	Arn     string                `json:"arn"`
	Streams map[string]*LogStream `json:"-"`
}

type MetricDimension struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type MetricDatum struct {
	MetricName string            `json:"MetricName"`
	Value      float64           `json:"Value"`
	Unit       string            `json:"Unit,omitempty"`
	Timestamp  float64           `json:"Timestamp,omitempty"` // Unix time epoch
	Dimensions []MetricDimension `json:"Dimensions,omitempty"`
}

type Metric struct {
	Namespace  string            `json:"Namespace"`
	MetricName string            `json:"MetricName"`
	Dimensions []MetricDimension `json:"Dimensions,omitempty"`
}

type CloudWatchHandler struct {
	mu        sync.RWMutex
	groups    map[string]*LogGroup
	metrics   map[string][]MetricDatum // Namespace -> list of data points
	AccountID string
}

func NewHandler() *CloudWatchHandler {
	return &CloudWatchHandler{
		groups:    make(map[string]*LogGroup),
		metrics:   make(map[string][]MetricDatum),
		AccountID: "000000000000",
	}
}

func (h *CloudWatchHandler) Name() string {
	return "cloudwatch"
}

func (h *CloudWatchHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 {
			lowerPart := strings.ToLower(parts[0])
			return strings.Contains(lowerPart, "logs") || strings.Contains(lowerPart, "granite") || strings.Contains(lowerPart, "cloudwatch")
		}
	}
	return false
}

func (h *CloudWatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, "CloudWatch")
		return
	}

	prefix := parts[0]
	action := parts[1]

	if prefix == logsTargetPrefix {
		h.handleLogs(w, r, action)
	} else if prefix == metricsTargetPrefix {
		h.handleMetrics(w, r, action)
	} else {
		awserr.New(400, "InvalidAction", fmt.Sprintf("Prefix %s not supported", prefix)).
			WriteJSONResponse(w, "CloudWatch")
	}
}

// Logs action routers
func (h *CloudWatchHandler) handleLogs(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "CreateLogGroup":
		h.handleCreateLogGroup(w, r)
	case "CreateLogStream":
		h.handleCreateLogStream(w, r)
	case "PutLogEvents":
		h.handlePutLogEvents(w, r)
	case "GetLogEvents":
		h.handleGetLogEvents(w, r)
	case "DescribeLogGroups":
		h.handleDescribeLogGroups(w, r)
	case "DescribeLogStreams":
		h.handleDescribeLogStreams(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action Logs.%s not supported", action)).
			WriteJSONResponse(w, logsTargetPrefix)
	}
}

// Metrics action routers
func (h *CloudWatchHandler) handleMetrics(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "PutMetricData":
		h.handlePutMetricData(w, r)
	case "ListMetrics":
		h.handleListMetrics(w, r)
	case "GetMetricStatistics":
		h.handleGetMetricStatistics(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action Metrics.%s not supported", action)).
			WriteJSONResponse(w, metricsTargetPrefix)
	}
}

// Logs Handlers
func (h *CloudWatchHandler) handleCreateLogGroup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LogGroupName string            `json:"logGroupName"`
		Tags         map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.groups[input.LogGroupName]; exists {
		awserr.New(400, "ResourceAlreadyExistsException", "Log group already exists").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	h.groups[input.LogGroupName] = &LogGroup{
		Name:    input.LogGroupName,
		Arn:     fmt.Sprintf("arn:aws:logs:us-east-1:%s:log-group:%s", h.AccountID, input.LogGroupName),
		Streams: make(map[string]*LogStream),
	}

	w.WriteHeader(http.StatusOK)
}

func (h *CloudWatchHandler) handleCreateLogStream(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LogGroupName  string `json:"logGroupName"`
		LogStreamName string `json:"logStreamName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	group, exists := h.groups[input.LogGroupName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log group not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	if _, exists := group.Streams[input.LogStreamName]; exists {
		awserr.New(400, "ResourceAlreadyExistsException", "Log stream already exists").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	group.Streams[input.LogStreamName] = &LogStream{
		Name:   input.LogStreamName,
		Arn:    fmt.Sprintf("arn:aws:logs:us-east-1:%s:log-group:%s:log-stream:%s", h.AccountID, input.LogGroupName, input.LogStreamName),
		Events: make([]LogEvent, 0),
	}

	w.WriteHeader(http.StatusOK)
}

func (h *CloudWatchHandler) handlePutLogEvents(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LogGroupName  string     `json:"logGroupName"`
		LogStreamName string     `json:"logStreamName"`
		LogEvents     []LogEvent `json:"logEvents"`
		SequenceToken string     `json:"sequenceToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	group, exists := h.groups[input.LogGroupName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log group not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	stream, exists := group.Streams[input.LogStreamName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log stream not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	stream.Events = append(stream.Events, input.LogEvents...)

	// Return mock next sequence token
	nextToken := fmt.Sprintf("token-%d", time.Now().UnixNano())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"nextSequenceToken": nextToken})
}

func (h *CloudWatchHandler) handleGetLogEvents(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LogGroupName  string `json:"logGroupName"`
		LogStreamName string `json:"logStreamName"`
		StartTime     int64  `json:"startTime"`
		EndTime       int64  `json:"endTime"`
		Limit         int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	group, exists := h.groups[input.LogGroupName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log group not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	stream, exists := group.Streams[input.LogStreamName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log stream not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	filtered := make([]LogEvent, 0)
	for _, event := range stream.Events {
		if input.StartTime > 0 && event.Timestamp < input.StartTime {
			continue
		}
		if input.EndTime > 0 && event.Timestamp > input.EndTime {
			continue
		}
		filtered = append(filtered, event)
	}

	// Apply limit
	limit := input.Limit
	if limit == 0 {
		limit = 10000
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"events":            filtered,
		"nextForwardToken":  "token-forward",
		"nextBackwardToken": "token-backward",
	})
}

func (h *CloudWatchHandler) handleDescribeLogGroups(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	groupsList := make([]*LogGroup, 0, len(h.groups))
	for _, g := range h.groups {
		groupsList = append(groupsList, g)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"logGroups": groupsList})
}

func (h *CloudWatchHandler) handleDescribeLogStreams(w http.ResponseWriter, r *http.Request) {
	var input struct {
		LogGroupName string `json:"logGroupName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	group, exists := h.groups[input.LogGroupName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "Log group not found").WriteJSONResponse(w, logsTargetPrefix)
		return
	}

	streamsList := make([]*LogStream, 0, len(group.Streams))
	for _, s := range group.Streams {
		streamsList = append(streamsList, s)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"logStreams": streamsList})
}

// Metrics Handlers
func (h *CloudWatchHandler) handlePutMetricData(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Namespace  string        `json:"Namespace"`
		MetricData []MetricDatum `json:"MetricData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	dataList := h.metrics[input.Namespace]
	nowEpoch := float64(time.Now().Unix())

	for _, datum := range input.MetricData {
		if datum.Timestamp == 0 {
			datum.Timestamp = nowEpoch
		}
		dataList = append(dataList, datum)
	}
	h.metrics[input.Namespace] = dataList

	w.WriteHeader(http.StatusOK)
}

func (h *CloudWatchHandler) handleListMetrics(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	metricsList := make([]Metric, 0)
	for namespace, dataPoints := range h.metrics {
		for _, dp := range dataPoints {
			metricsList = append(metricsList, Metric{
				Namespace:  namespace,
				MetricName: dp.MetricName,
				Dimensions: dp.Dimensions,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"Metrics": metricsList})
}

func (h *CloudWatchHandler) handleGetMetricStatistics(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Namespace  string   `json:"Namespace"`
		MetricName string   `json:"MetricName"`
		StartTime  float64  `json:"StartTime"`
		EndTime    float64  `json:"EndTime"`
		Period     int      `json:"Period"`
		Statistics []string `json:"Statistics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	dataPoints, exists := h.metrics[input.Namespace]
	h.mu.RUnlock()

	type StatResult struct {
		Timestamp  string             `json:"Timestamp"`
		Sum        float64            `json:"Sum,omitempty"`
		Average    float64            `json:"Average,omitempty"`
		Minimum    float64            `json:"Minimum,omitempty"`
		Maximum    float64            `json:"Maximum,omitempty"`
		SampleCount float64           `json:"SampleCount,omitempty"`
		Unit       string             `json:"Unit,omitempty"`
	}

	results := make([]StatResult, 0)
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"Datapoints": results})
		return
	}

	// Filter data points in range
	matchingPoints := make([]MetricDatum, 0)
	for _, dp := range dataPoints {
		if dp.MetricName == input.MetricName && dp.Timestamp >= input.StartTime && dp.Timestamp <= input.EndTime {
			matchingPoints = append(matchingPoints, dp)
		}
	}

	if len(matchingPoints) > 0 {
		// Group by simple period buckets
		var sum, min, max float64
		min = matchingPoints[0].Value
		max = matchingPoints[0].Value
		for _, dp := range matchingPoints {
			sum += dp.Value
			if dp.Value < min {
				min = dp.Value
			}
			if dp.Value > max {
				max = dp.Value
			}
		}
		count := float64(len(matchingPoints))
		avg := sum / count

		results = append(results, StatResult{
			Timestamp:   time.Unix(int64(input.StartTime), 0).Format(time.RFC3339),
			Sum:         sum,
			Average:     avg,
			Minimum:     min,
			Maximum:     max,
			SampleCount: count,
			Unit:        matchingPoints[0].Unit,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"Datapoints": results})
}

type LogStreamSnapshot struct {
	Name   string     `json:"name"`
	Arn    string     `json:"arn"`
	Events []LogEvent `json:"events"`
}

type LogGroupSnapshot struct {
	Name    string               `json:"name"`
	Arn     string               `json:"arn"`
	Streams []LogStreamSnapshot `json:"streams"`
}

func (h *CloudWatchHandler) GetLogGroups() []LogGroupSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]LogGroupSnapshot, 0, len(h.groups))
	for _, g := range h.groups {
		streams := make([]LogStreamSnapshot, 0, len(g.Streams))
		for _, s := range g.Streams {
			streams = append(streams, LogStreamSnapshot{
				Name:   s.Name,
				Arn:    s.Arn,
				Events: s.Events,
			})
		}
		res = append(res, LogGroupSnapshot{
			Name:    g.Name,
			Arn:     g.Arn,
			Streams: streams,
		})
	}
	return res
}
