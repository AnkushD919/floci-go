package apigatewayv2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/floci-io/floci-go/internal/services/lambda"
)

func TestAPIGatewayV2Lifecycle(t *testing.T) {
	lambdaH := lambda.NewHandler()
	handler := NewHandler(lambdaH, "4566")

	// 1. Create API
	createApiInput := map[string]string{
		"Name":         "my-http-api",
		"ProtocolType": "HTTP",
	}
	bodyBytes, _ := json.Marshal(createApiInput)
	req := httptest.NewRequest(http.MethodPost, "/v2/apis", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected CreateApi status 201, got %d", resp.StatusCode)
	}

	var api API
	_ = json.NewDecoder(resp.Body).Decode(&api)
	if api.Name != "my-http-api" || api.ApiID == "" {
		t.Fatalf("invalid API output: %+v", api)
	}

	// 2. Create Integration
	createIntInput := map[string]string{
		"IntegrationType":      "AWS_PROXY",
		"IntegrationUri":       "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/arn:aws:lambda:us-east-1:000000000000:function:test-func/invocations",
		"PayloadFormatVersion": "2.0",
	}
	bodyBytes, _ = json.Marshal(createIntInput)
	req = httptest.NewRequest(http.MethodPost, "/v2/apis/"+api.ApiID+"/integrations", bytes.NewReader(bodyBytes))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected CreateIntegration status 201, got %d", resp.StatusCode)
	}

	var integration Integration
	_ = json.NewDecoder(resp.Body).Decode(&integration)
	if integration.IntegrationID == "" || integration.IntegrationType != "AWS_PROXY" {
		t.Fatalf("invalid Integration output: %+v", integration)
	}

	// 3. Create Route
	createRouteInput := map[string]string{
		"RouteKey": "GET /users",
		"Target":   "integrations/" + integration.IntegrationID,
	}
	bodyBytes, _ = json.Marshal(createRouteInput)
	req = httptest.NewRequest(http.MethodPost, "/v2/apis/"+api.ApiID+"/routes", bytes.NewReader(bodyBytes))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected CreateRoute status 201, got %d", resp.StatusCode)
	}

	var route Route
	_ = json.NewDecoder(resp.Body).Decode(&route)
	if route.RouteID == "" || route.RouteKey != "GET /users" {
		t.Fatalf("invalid Route output: %+v", route)
	}

	// 4. Get APIs list
	req = httptest.NewRequest(http.MethodGet, "/v2/apis", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var getApisOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&getApisOutput)
	items, ok := getApisOutput["Items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Errorf("expected 1 API in list, got %d", len(items))
	}
}
