package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/floci-io/floci-go/internal/config"
	"github.com/floci-io/floci-go/internal/services/apigatewayv2"
	"github.com/floci-io/floci-go/internal/services/cloudwatch"
	"github.com/floci-io/floci-go/internal/services/cognito"
	"github.com/floci-io/floci-go/internal/services/dynamodb"
	"github.com/floci-io/floci-go/internal/services/eventbridge"
	"github.com/floci-io/floci-go/internal/services/iam"
	"github.com/floci-io/floci-go/internal/services/kms"
	"github.com/floci-io/floci-go/internal/services/lambda"
	"github.com/floci-io/floci-go/internal/services/rds"
	"github.com/floci-io/floci-go/internal/services/s3"
	"github.com/floci-io/floci-go/internal/services/secretsmanager"
	"github.com/floci-io/floci-go/internal/services/sns"
	"github.com/floci-io/floci-go/internal/services/ssm"
	"github.com/floci-io/floci-go/internal/services/stepfunctions"
)

func TestUIConsoleHandlers(t *testing.T) {
	cfg := &config.Config{Port: "4566"}
	startTime := time.Now()

	iamH := iam.NewHandler()
	ssmH := ssm.NewHandler()
	smH := secretsmanager.NewHandler()
	kmsH := kms.NewHandler()
	snsH := sns.NewHandler()
	s3H := s3.NewHandler()
	ddbH := dynamodb.NewHandler()
	lambdaH := lambda.NewHandler()
	apigwH := apigatewayv2.NewHandler(lambdaH, cfg.Port)
	eventbH := eventbridge.NewHandler(lambdaH)
	cognitoH := cognito.NewHandler()
	cwH := cloudwatch.NewHandler()
	rdsH := rds.NewHandler()
	sfH := stepfunctions.NewHandler(lambdaH)

	handler := NewHandler(startTime, cfg, iamH, ssmH, smH, kmsH, snsH, s3H, ddbH, lambdaH, apigwH, eventbH, cognitoH, cwH, rdsH, sfH)

	// 1. Test /_floci/health
	req := httptest.NewRequest(http.MethodGet, "/_floci/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected health code 200, got %d", w.Code)
	}
	var health map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &health)
	if health["status"] != "ready" {
		t.Errorf("expected health status 'ready', got %v", health["status"])
	}

	// 2. Test /_floci/info
	req = httptest.NewRequest(http.MethodGet, "/_floci/info", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var info map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	if info["port"] != "4566" {
		t.Errorf("expected port '4566', got %v", info["port"])
	}

	// 3. Test /_floci/init
	req = httptest.NewRequest(http.MethodGet, "/_floci/init", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var initResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &initResp)
	completed := initResp["completed"].(map[string]interface{})
	if completed["ready"] != true {
		t.Errorf("expected completed ready true")
	}

	// 4. Test /_floci/api/resources
	req = httptest.NewRequest(http.MethodGet, "/_floci/api/resources", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var resources map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resources)
	if _, ok := resources["s3"]; !ok {
		t.Errorf("expected s3 resources key")
	}

	// 5. Test static console page serve
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected static landing page serve code 200, got %d", w.Code)
	}
	bodyStr := w.Body.String()
	if !testing.Short() && len(bodyStr) == 0 {
		t.Errorf("expected non-empty landing page content")
	}
}
