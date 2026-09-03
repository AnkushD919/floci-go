package ui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
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
	"github.com/floci-io/floci-go/internal/services/sqs"
	"github.com/floci-io/floci-go/internal/services/ssm"
	"github.com/floci-io/floci-go/internal/services/stepfunctions"
)

//go:embed all:web
var webAssets embed.FS

type UIHandler struct {
	startTime       time.Time
	cfg             *config.Config
	iamHandler      *iam.IAMHandler
	ssmHandler      *ssm.SSMHandler
	smHandler       *secretsmanager.SecretsManagerHandler
	kmsHandler      *kms.KMSHandler
	snsHandler      *sns.SNSHandler
	s3Handler       *s3.S3Handler
	dynamodbHandler *dynamodb.DynamoDBHandler
	lambdaHandler   *lambda.LambdaHandler
	apigwHandler    *apigatewayv2.APIGatewayV2Handler
	eventbHandler   *eventbridge.EventBridgeHandler
	cognitoHandler  *cognito.CognitoHandler
	cwHandler       *cloudwatch.CloudWatchHandler
	rdsHandler      *rds.RDSHandler
	sfHandler       *stepfunctions.StepFunctionsHandler
}

func (h *UIHandler) Name() string {
	return "ui"
}

func (h *UIHandler) Matches(r *http.Request) bool {
	path := r.URL.Path
	if path == "/index.html" || path == "/starting.html" || strings.HasPrefix(path, "/_floci/") {
		return true
	}
	if path == "/" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		return true
	}
	return false
}

func NewHandler(
	startTime time.Time,
	cfg *config.Config,
	iamHandler *iam.IAMHandler,
	ssmHandler *ssm.SSMHandler,
	smHandler *secretsmanager.SecretsManagerHandler,
	kmsHandler *kms.KMSHandler,
	snsHandler *sns.SNSHandler,
	s3Handler *s3.S3Handler,
	dynamodbHandler *dynamodb.DynamoDBHandler,
	lambdaHandler *lambda.LambdaHandler,
	apigwHandler *apigatewayv2.APIGatewayV2Handler,
	eventbHandler *eventbridge.EventBridgeHandler,
	cognitoHandler *cognito.CognitoHandler,
	cwHandler *cloudwatch.CloudWatchHandler,
	rdsHandler *rds.RDSHandler,
	sfHandler *stepfunctions.StepFunctionsHandler,
) *UIHandler {
	return &UIHandler{
		startTime:       startTime,
		cfg:             cfg,
		iamHandler:      iamHandler,
		ssmHandler:      ssmHandler,
		smHandler:       smHandler,
		kmsHandler:      kmsHandler,
		snsHandler:      snsHandler,
		s3Handler:       s3Handler,
		dynamodbHandler: dynamodbHandler,
		lambdaHandler:   lambdaHandler,
		apigwHandler:    apigwHandler,
		eventbHandler:   eventbHandler,
		cognitoHandler:  cognitoHandler,
		cwHandler:       cwHandler,
		rdsHandler:      rdsHandler,
		sfHandler:       sfHandler,
	}
}

func (h *UIHandler) ServeStatic(w http.ResponseWriter, r *http.Request) {
	// Strip "/web" prefix and use sub filesystem
	subFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Serve SSPA style index.html fallback for root or other routes
	if r.URL.Path == "/" || r.URL.Path == "" {
		content, err := fs.ReadFile(subFS, "index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}

	// Serve starting page
	if r.URL.Path == "/starting.html" {
		content, err := fs.ReadFile(subFS, "starting.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}

	http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
}

func (h *UIHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	uptime := time.Since(h.startTime).Truncate(time.Second).String()
	resp := map[string]interface{}{
		"status":  "ready",
		"version": "0.0.1",
		"edition": "community",
		"uptime":  uptime,
		"services": map[string]string{
			"sts":            "running",
			"iam":            "running",
			"ssm":            "running",
			"secretsmanager": "running",
			"kms":            "running",
			"sqs":            "running",
			"sns":            "running",
			"s3":             "running",
			"dynamodb":       "running",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *UIHandler) HandleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"port":      h.cfg.Port,
		"region":    "us-east-1",
		"accountId": "000000000000",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *UIHandler) HandleInit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"completed": map[string]bool{
			"boot":  true,
			"start": true,
			"ready": true,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *UIHandler) HandleResourcesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get SQS snapshot from GlobalRegistry
	sqsQueues := sqs.GlobalRegistry.GetQueues()
	sqsSnapshots := make([]map[string]interface{}, 0, len(sqsQueues))
	for _, q := range sqsQueues {
		msgs := make([]map[string]interface{}, 0, len(q.Messages))
		for _, m := range q.Messages {
			msgs = append(msgs, map[string]interface{}{
				"MessageId":     m.MessageID,
				"ReceiptHandle": m.ReceiptHandle,
				"Body":          m.Body,
				"MD5OfBody":     m.MD5OfBody,
			})
		}
		sqsSnapshots = append(sqsSnapshots, map[string]interface{}{
			"Name":         q.Name,
			"URL":          q.URL,
			"ARN":          q.ARN,
			"messageCount": len(q.Messages),
			"Messages":     msgs,
		})
	}

	// DynamoDB Tables
	ddbTables := h.dynamodbHandler.GetTables()
	ddbSnapshots := make([]map[string]interface{}, 0, len(ddbTables))
	for _, t := range ddbTables {
		ddbSnapshots = append(ddbSnapshots, map[string]interface{}{
			"Meta":  t.Meta,
			"Items": t.Items,
		})
	}

	// Compile complete system resource snapshot
	resp := map[string]interface{}{
		"s3": map[string]interface{}{
			"buckets": h.s3Handler.GetBuckets(),
		},
		"dynamodb": map[string]interface{}{
			"tables": ddbSnapshots,
		},
		"sqs": map[string]interface{}{
			"queues": sqsSnapshots,
		},
		"sns": map[string]interface{}{
			"topics":        h.snsHandler.GetTopics(),
			"subscriptions": h.snsHandler.GetSubscriptions(),
		},
		"ssm": map[string]interface{}{
			"parameters": h.ssmHandler.GetParameters(),
		},
		"secretsmanager": map[string]interface{}{
			"secrets": h.smHandler.GetSecrets(),
		},
		"iam": map[string]interface{}{
			"roles": h.iamHandler.GetRoles(),
		},
		"kms": map[string]interface{}{
			"keys": h.kmsHandler.GetKeys(),
		},
		"lambda": map[string]interface{}{
			"functions": h.lambdaHandler.GetFunctions(),
		},
		"apigatewayv2": map[string]interface{}{
			"apis": h.apigwHandler.GetApis(),
		},
		"eventbridge": map[string]interface{}{
			"rules": h.eventbHandler.GetRules(),
		},
		"cognito": map[string]interface{}{
			"pools": h.cognitoHandler.GetPools(),
		},
		"cloudwatch": map[string]interface{}{
			"groups": h.cwHandler.GetLogGroups(),
		},
		"rds": map[string]interface{}{
			"instances": h.rdsHandler.GetInstances(),
		},
		"stepfunctions": map[string]interface{}{
			"stateMachines": h.sfHandler.GetStateMachines(),
			"executions":    h.sfHandler.GetExecutions(),
		},
	}
	
	_ = json.NewEncoder(w).Encode(resp)
}

// ServeHTTP implements Route interception for the Console UI
func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/_floci/health":
		h.HandleHealth(w, r)
	case "/_floci/info":
		h.HandleInfo(w, r)
	case "/_floci/init":
		h.HandleInit(w, r)
	case "/_floci/api/resources":
		h.HandleResourcesAPI(w, r)
	default:
		h.ServeStatic(w, r)
	}
}
