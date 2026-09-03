package apigatewayv2

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/floci-io/floci-go/internal/awserr"
	"github.com/floci-io/floci-go/internal/services/lambda"
)

type Route struct {
	RouteID  string `json:"RouteId"`
	RouteKey string `json:"RouteKey"` // e.g. "GET /users" or "$default"
	Target   string `json:"Target"`   // e.g. "integrations/integrationId"
}

type Integration struct {
	IntegrationID        string `json:"IntegrationId"`
	IntegrationType      string `json:"IntegrationType"` // e.g. "AWS_PROXY"
	IntegrationUri       string `json:"IntegrationUri"`  // e.g. Lambda ARN
	PayloadFormatVersion string `json:"PayloadFormatVersion"`
}

type API struct {
	ApiID        string                 `json:"ApiId"`
	Name         string                 `json:"Name"`
	ProtocolType string                 `json:"ProtocolType"` // e.g. "HTTP"
	ApiEndpoint  string                 `json:"ApiEndpoint"`
	Routes       map[string]*Route      // RouteKey -> Route
	Integrations map[string]*Integration // IntegrationID -> Integration
}

type APIGatewayV2Handler struct {
	mu            sync.RWMutex
	apis          map[string]*API
	lambdaHandler *lambda.LambdaHandler
	AccountID     string
	Port          string
}

func NewHandler(lambdaHandler *lambda.LambdaHandler, port string) *APIGatewayV2Handler {
	return &APIGatewayV2Handler{
		apis:          make(map[string]*API),
		lambdaHandler: lambdaHandler,
		AccountID:     "000000000000",
		Port:          port,
	}
}

func (h *APIGatewayV2Handler) Name() string {
	return "apigatewayv2"
}

func (h *APIGatewayV2Handler) Matches(r *http.Request) bool {
	// Match control plane
	if strings.HasPrefix(r.URL.Path, "/v2/apis") {
		return true
	}
	// Match data plane (deployed API routes)
	apiId, _, _, _ := h.resolveRoute(r)
	return apiId != ""
}

func (h *APIGatewayV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If it's a deployed route (data plane), proxy to Lambda
	apiId, _, routePath, method := h.resolveRoute(r)
	if apiId != "" {
		h.handleDataPlaneRequest(w, r, apiId, routePath, method)
		return
	}

	// Control plane APIs
	path := r.URL.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// POST /v2/apis
	// GET /v2/apis
	if len(parts) == 2 && parts[0] == "v2" && parts[1] == "apis" {
		if r.Method == http.MethodPost {
			h.handleCreateApi(w, r)
		} else if r.Method == http.MethodGet {
			h.handleGetApis(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) >= 3 && parts[0] == "v2" && parts[1] == "apis" {
		apiId := parts[2]
		subAction := ""
		if len(parts) >= 4 {
			subAction = parts[3]
		}

		if subAction == "routes" {
			if r.Method == http.MethodPost {
				h.handleCreateRoute(w, r, apiId)
			} else if r.Method == http.MethodGet {
				h.handleGetRoutes(w, r, apiId)
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		if subAction == "integrations" {
			if r.Method == http.MethodPost {
				h.handleCreateIntegration(w, r, apiId)
			} else if r.Method == http.MethodGet {
				h.handleGetIntegrations(w, r, apiId)
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		if subAction == "" && r.Method == http.MethodDelete {
			h.handleDeleteApi(w, r, apiId)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func (h *APIGatewayV2Handler) handleCreateApi(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string `json:"Name"`
		ProtocolType string `json:"ProtocolType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	apiId := "api-" + randomHex(4)
	endpoint := fmt.Sprintf("http://localhost:%s/%s", h.Port, apiId)

	api := &API{
		ApiID:        apiId,
		Name:         input.Name,
		ProtocolType: input.ProtocolType,
		ApiEndpoint:  endpoint,
		Routes:       make(map[string]*Route),
		Integrations: make(map[string]*Integration),
	}

	h.apis[apiId] = api

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(api)
}

func (h *APIGatewayV2Handler) handleGetApis(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*API, 0, len(h.apis))
	for _, api := range h.apis {
		list = append(list, api)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"Items": list})
}

func (h *APIGatewayV2Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request, apiId string) {
	h.mu.Lock()
	api, exists := h.apis[apiId]
	h.mu.Unlock()

	if !exists {
		awserr.New(404, "NotFoundException", "API not found").WriteJSONResponse(w, "APIGatewayV2")
		return
	}

	var input struct {
		RouteKey string `json:"RouteKey"`
		Target   string `json:"Target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	routeId := "route-" + randomHex(4)
	route := &Route{
		RouteID:  routeId,
		RouteKey: input.RouteKey,
		Target:   input.Target,
	}

	api.Routes[input.RouteKey] = route

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(route)
}

func (h *APIGatewayV2Handler) handleGetRoutes(w http.ResponseWriter, r *http.Request, apiId string) {
	h.mu.RLock()
	api, exists := h.apis[apiId]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "NotFoundException", "API not found").WriteJSONResponse(w, "APIGatewayV2")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*Route, 0, len(api.Routes))
	for _, route := range api.Routes {
		list = append(list, route)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"Items": list})
}

func (h *APIGatewayV2Handler) handleCreateIntegration(w http.ResponseWriter, r *http.Request, apiId string) {
	h.mu.Lock()
	api, exists := h.apis[apiId]
	h.mu.Unlock()

	if !exists {
		awserr.New(404, "NotFoundException", "API not found").WriteJSONResponse(w, "APIGatewayV2")
		return
	}

	var input struct {
		IntegrationType      string `json:"IntegrationType"`
		IntegrationUri       string `json:"IntegrationUri"`
		PayloadFormatVersion string `json:"PayloadFormatVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	intId := "integration-" + randomHex(4)
	version := input.PayloadFormatVersion
	if version == "" {
		version = "2.0"
	}

	integration := &Integration{
		IntegrationID:        intId,
		IntegrationType:      input.IntegrationType,
		IntegrationUri:       input.IntegrationUri,
		PayloadFormatVersion: version,
	}

	api.Integrations[intId] = integration

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(integration)
}

func (h *APIGatewayV2Handler) handleGetIntegrations(w http.ResponseWriter, r *http.Request, apiId string) {
	h.mu.RLock()
	api, exists := h.apis[apiId]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "NotFoundException", "API not found").WriteJSONResponse(w, "APIGatewayV2")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*Integration, 0, len(api.Integrations))
	for _, integration := range api.Integrations {
		list = append(list, integration)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"Items": list})
}

func (h *APIGatewayV2Handler) handleDeleteApi(w http.ResponseWriter, r *http.Request, apiId string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.apis[apiId]; !exists {
		awserr.New(404, "NotFoundException", "API not found").WriteJSONResponse(w, "APIGatewayV2")
		return
	}

	delete(h.apis, apiId)
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIGatewayV2Handler) resolveRoute(r *http.Request) (apiId string, stage string, routePath string, method string) {
	host := strings.ToLower(r.Host)
	method = r.Method

	if idx := strings.Index(host, ".execute-api"); idx != -1 {
		apiId = host[:idx]
		path := r.URL.Path
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) > 0 {
			stage = parts[0]
			routePath = "/" + strings.Join(parts[1:], "/")
		} else {
			routePath = "/"
		}
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		potentialApiId := parts[0]
		h.mu.RLock()
		_, exists := h.apis[potentialApiId]
		h.mu.RUnlock()
		if exists {
			apiId = potentialApiId
			if len(parts) > 1 {
				stage = parts[1]
				routePath = "/" + strings.Join(parts[2:], "/")
			} else {
				routePath = "/"
			}
			return
		}
	}
	return "", "", "", ""
}

func (h *APIGatewayV2Handler) handleDataPlaneRequest(w http.ResponseWriter, r *http.Request, apiId string, routePath string, method string) {
	h.mu.RLock()
	api, exists := h.apis[apiId]
	h.mu.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("API not found"))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Find matching route key (e.g. "GET /users" or "ANY /users" or "$default")
	var matchedRoute *Route
	routeKeysToTry := []string{
		fmt.Sprintf("%s %s", method, routePath),
		fmt.Sprintf("ANY %s", routePath),
		"$default",
	}

	for _, key := range routeKeysToTry {
		if route, exists := api.Routes[key]; exists {
			matchedRoute = route
			break
		}
	}

	if matchedRoute == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(fmt.Sprintf("No route matches %s %s", method, routePath)))
		return
	}

	// Resolve integration target (e.g. "integrations/integrationId")
	intId := strings.TrimPrefix(matchedRoute.Target, "integrations/")
	integration, exists := api.Integrations[intId]
	if !exists {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Route lacks a valid integration target"))
		return
	}

	// Execute integrated Lambda function
	funcName := extractFunctionName(integration.IntegrationUri)

	// Construct API Gateway Event JSON (Payload v2.0 format)
	bodyBytes, _ := io.ReadAll(r.Body)
	isBase64 := false
	bodyStr := string(bodyBytes)
	if len(bodyBytes) > 0 && !isUTF8(bodyBytes) {
		bodyStr = base64.StdEncoding.EncodeToString(bodyBytes)
		isBase64 = true
	}

	headersMap := make(map[string]string)
	for k, vv := range r.Header {
		if len(vv) > 0 {
			headersMap[strings.ToLower(k)] = vv[0]
		}
	}

	event := map[string]interface{}{
		"version":        "2.0",
		"routeKey":       matchedRoute.RouteKey,
		"rawPath":        routePath,
		"rawQueryString": r.URL.RawQuery,
		"headers":        headersMap,
		"requestContext": map[string]interface{}{
			"accountId":    h.AccountID,
			"apiId":        apiId,
			"domainName":   r.Host,
			"domainPrefix": apiId,
			"http": map[string]interface{}{
				"method":    method,
				"path":      routePath,
				"protocol":  r.Proto,
				"sourceIp":  r.RemoteAddr,
				"userAgent": r.UserAgent(),
			},
		},
		"body":            bodyStr,
		"isBase64Encoded": isBase64,
	}

	eventBytes, _ := json.Marshal(event)

	// Create request recording recorder to intercept response from Lambda
	lambdaRec := httptest.NewRecorder()
	lambdaReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/2015-03-31/functions/%s/invocations", funcName), bytes.NewReader(eventBytes))

	h.lambdaHandler.ServeHTTP(lambdaRec, lambdaReq)

	lambdaResp := lambdaRec.Result()
	if lambdaResp.StatusCode != http.StatusOK {
		w.WriteHeader(lambdaResp.StatusCode)
		_, _ = io.Copy(w, lambdaResp.Body)
		return
	}

	// Parse Lambda JSON proxy integration response
	var proxyResp struct {
		StatusCode      int               `json:"statusCode"`
		Headers         map[string]string `json:"headers"`
		Body            string            `json:"body"`
		IsBase64Encoded bool              `json:"isBase64Encoded"`
	}

	if err := json.NewDecoder(lambdaResp.Body).Decode(&proxyResp); err != nil {
		// Fallback: if not structured proxy JSON, return raw body as string
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, lambdaResp.Body)
		return
	}

	// Write custom headers
	for k, v := range proxyResp.Headers {
		w.Header().Set(k, v)
	}

	statusCode := proxyResp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	w.WriteHeader(statusCode)

	if proxyResp.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(proxyResp.Body)
		if err == nil {
			_, _ = w.Write(decoded)
		} else {
			_, _ = w.Write([]byte(proxyResp.Body))
		}
	} else {
		_, _ = w.Write([]byte(proxyResp.Body))
	}
}

func extractFunctionName(uri string) string {
	// e.g. arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:000000000000:function:test-func/invocations
	if idx := strings.Index(uri, ":function:"); idx != -1 {
		part := uri[idx+len(":function:"):]
		if slashIdx := strings.Index(part, "/"); slashIdx != -1 {
			return part[:slashIdx]
		}
		return part
	}
	parts := strings.Split(uri, ":")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if slashIdx := strings.Index(last, "/"); slashIdx != -1 {
			return last[:slashIdx]
		}
		return last
	}
	return uri
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func isUTF8(b []byte) bool {
	return strings.ToValidUTF8(string(b), "") == string(b)
}

func (h *APIGatewayV2Handler) GetApis() []*API {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*API, 0, len(h.apis))
	for _, api := range h.apis {
		list = append(list, api)
	}
	return list
}
