package rds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRDSLifecycle(t *testing.T) {
	_ = os.Remove(filepath.Join(os.TempDir(), "floci-rds", "mydb.db"))

	handler := NewHandler()

	// 1. CreateDBInstance
	createInput := map[string]string{
		"DBInstanceIdentifier": "test-db",
		"Engine":               "sqlite",
		"DBName":               "mydb",
	}
	bodyBytes, _ := json.Marshal(createInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonRDSv12.CreateDBInstance")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected CreateDBInstance status 200, got %d", resp.StatusCode)
	}

	// 2. DescribeDBInstances
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{}")))
	req.Header.Set("X-Amz-Target", "AmazonRDSv12.DescribeDBInstances")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected DescribeDBInstances status 200, got %d", resp.StatusCode)
	}

	var descOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&descOutput)
	list, ok := descOutput["DBInstances"].([]interface{})
	if !ok || len(list) != 1 {
		t.Errorf("expected 1 DB instance, got %d", len(list))
	}

	// 3. ExecuteStatement: Create Table
	createTableInput := ExecuteStatementInput{
		Database:    "mydb",
		ResourceArn: "arn:aws:rds:us-east-1:000000000000:cluster:test-db",
		Sql:         "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER);",
	}
	bodyBytes, _ = json.Marshal(createTableInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "RDSDataService.ExecuteStatement")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected ExecuteStatement status 200, got %d. Body: %s", resp.StatusCode, buf.String())
	}

	// 4. ExecuteStatement: Insert item using SQL parameters
	nameVal := "Alice"
	var ageVal int64 = 25
	insertInput := ExecuteStatementInput{
		Database:    "mydb",
		ResourceArn: "arn:aws:rds:us-east-1:000000000000:cluster:test-db",
		Sql:         "INSERT INTO users (name, age) VALUES (:name, :age);",
		Parameters: []SqlParameter{
			{Name: "name", Value: SqlParameterValue{StringValue: &nameVal}},
			{Name: "age", Value: SqlParameterValue{LongValue: &ageVal}},
		},
	}
	bodyBytes, _ = json.Marshal(insertInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "RDSDataService.ExecuteStatement")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected Insert status 200, got %d. Body: %s", resp.StatusCode, buf.String())
	}

	// 5. ExecuteStatement: Query items
	queryInput := ExecuteStatementInput{
		Database:    "mydb",
		ResourceArn: "arn:aws:rds:us-east-1:000000000000:cluster:test-db",
		Sql:         "SELECT id, name, age FROM users WHERE name = :name;",
		Parameters: []SqlParameter{
			{Name: "name", Value: SqlParameterValue{StringValue: &nameVal}},
		},
	}
	bodyBytes, _ = json.Marshal(queryInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "RDSDataService.ExecuteStatement")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("expected SELECT status 200, got %d. Body: %s", resp.StatusCode, buf.String())
	}

	var queryOutput ExecuteStatementOutput
	_ = json.NewDecoder(resp.Body).Decode(&queryOutput)
	if len(queryOutput.Records) != 1 {
		t.Fatalf("expected 1 record returned, got %d", len(queryOutput.Records))
	}
	row := queryOutput.Records[0]
	if *row[1].StringValue != "Alice" || *row[2].LongValue != 25 {
		t.Errorf("unexpected query result fields: %+v", row)
	}

	// 6. DeleteDBInstance
	deleteInput := map[string]string{
		"DBInstanceIdentifier": "test-db",
	}
	bodyBytes, _ = json.Marshal(deleteInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonRDSv12.DeleteDBInstance")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected DeleteDBInstance status 200, got %d", resp.StatusCode)
	}
}
