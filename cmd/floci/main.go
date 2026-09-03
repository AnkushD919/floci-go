package main

import (
	"log"
	"net/http"
	"time"

	"github.com/floci-io/floci-go/internal/config"
	"github.com/floci-io/floci-go/internal/router"
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
	"github.com/floci-io/floci-go/internal/services/sts"
	"github.com/floci-io/floci-go/internal/ui"
)

func main() {
	startTime := time.Now()
	cfg := config.Load()

	log.Printf("Starting floci-go Local AWS Emulator v0.0.1...")
	log.Printf("Port: %s", cfg.Port)
	log.Printf("Storage Mode: %s", cfg.Storage)
	log.Printf("Log Level: %s", cfg.LogLevel)

	r := router.New()

	// Instantiate individual handlers to expose references to UI console
	stsH := sts.NewHandler()
	iamH := iam.NewHandler()
	ssmH := ssm.NewHandler()
	smH := secretsmanager.NewHandler()
	kmsH := kms.NewHandler()
	sqsH := sqs.NewHandler()
	snsH := sns.NewHandler()
	s3H := s3.NewHandler()
	ddbH := dynamodb.NewHandler()

	// Instantiate Sprint 1 handlers
	lambdaH := lambda.NewHandler()
	apigwH := apigatewayv2.NewHandler(lambdaH, cfg.Port)
	eventbH := eventbridge.NewHandler(lambdaH)

	// Instantiate Sprint 2 handlers
	cognitoH := cognito.NewHandler()
	cwH := cloudwatch.NewHandler()

	// Instantiate Sprint 3 handlers
	rdsH := rds.NewHandler()
	sfH := stepfunctions.NewHandler(lambdaH)

	uiH := ui.NewHandler(startTime, cfg, iamH, ssmH, smH, kmsH, snsH, s3H, ddbH, lambdaH, apigwH, eventbH, cognitoH, cwH, rdsH, sfH)

	// Register service plugins in precedence order. S3 is registered last as a fallback.
	r.RegisterPlugin(uiH)
	r.RegisterPlugin(apigwH)
	r.RegisterPlugin(lambdaH)
	r.RegisterPlugin(eventbH)
	r.RegisterPlugin(cognitoH)
	r.RegisterPlugin(cwH)
	r.RegisterPlugin(rdsH)
	r.RegisterPlugin(sfH)
	r.RegisterPlugin(stsH)
	r.RegisterPlugin(iamH)
	r.RegisterPlugin(ssmH)
	r.RegisterPlugin(smH)
	r.RegisterPlugin(kmsH)
	r.RegisterPlugin(sqsH)
	r.RegisterPlugin(snsH)
	r.RegisterPlugin(ddbH)
	r.RegisterPlugin(s3H)

	address := ":" + cfg.Port
	log.Printf("floci-go listening on http://localhost%s", address)
	srv := &http.Server{
		Addr:         address,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 5 * time.Minute, // Lambda cold-starts or step function execution might be slow
		IdleTimeout:  120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start emulator: %v", err)
	}
}
