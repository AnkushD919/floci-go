package dynamodb

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDynamoDBLifecycle(t *testing.T) {
	handler := NewHandler()

	tableName := "test-table"

	// 1. Create Table
	createInput := CreateTableInput{
		TableName: tableName,
		AttributeDefinitions: []AttributeDefinition{
			{AttributeName: "pk", AttributeType: "S"},
			{AttributeName: "sk", AttributeType: "S"},
		},
		KeySchema: []KeySchemaElement{
			{AttributeName: "pk", KeyType: "HASH"},
			{AttributeName: "sk", KeyType: "RANGE"},
		},
		BillingMode: "PAY_PER_REQUEST",
	}
	bodyBytes, _ := json.Marshal(createInput)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.CreateTable")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateTable status 200, got %d", resp.StatusCode)
	}

	var createOutput CreateTableOutput
	_ = json.NewDecoder(resp.Body).Decode(&createOutput)
	if createOutput.TableDescription.TableName != tableName {
		t.Errorf("expected table name %s, got %s", tableName, createOutput.TableDescription.TableName)
	}

	// 2. Describe Table
	descInput := DescribeTableInput{
		TableName: tableName,
	}
	bodyBytes, _ = json.Marshal(descInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.DescribeTable")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var descOutput DescribeTableOutput
	_ = json.NewDecoder(resp.Body).Decode(&descOutput)
	if descOutput.Table.TableStatus != "ACTIVE" {
		t.Errorf("expected TableStatus ACTIVE, got %s", descOutput.Table.TableStatus)
	}

	// 3. Put Item
	item := map[string]interface{}{
		"pk":   map[string]interface{}{"S": "user#1"},
		"sk":   map[string]interface{}{"S": "profile"},
		"name": map[string]interface{}{"S": "Alice"},
	}
	putInput := PutItemInput{
		TableName: tableName,
		Item:      item,
	}
	bodyBytes, _ = json.Marshal(putInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.PutItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected PutItem status 200, got %d", resp.StatusCode)
	}

	// 4. Get Item
	key := map[string]interface{}{
		"pk": map[string]interface{}{"S": "user#1"},
		"sk": map[string]interface{}{"S": "profile"},
	}
	getInput := GetItemInput{
		TableName: tableName,
		Key:       key,
	}
	bodyBytes, _ = json.Marshal(getInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.GetItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var getOutput GetItemOutput
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	nameAttr, ok := getOutput.Item["name"].(map[string]interface{})
	if !ok || nameAttr["S"] != "Alice" {
		t.Errorf("expected item name 'Alice', got %v", getOutput.Item["name"])
	}

	// 5. Update Item
	updateInput := UpdateItemInput{
		TableName: tableName,
		Key:       key,
		UpdateExpression: "SET #n = :n",
		ExpressionAttributeNames: map[string]string{
			"#n": "name",
		},
		ExpressionAttributeValues: map[string]interface{}{
			":n": map[string]interface{}{"S": "Alice Updated"},
		},
	}
	bodyBytes, _ = json.Marshal(updateInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.UpdateItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected UpdateItem status 200, got %d", resp.StatusCode)
	}

	// Verify Update
	bodyBytes, _ = json.Marshal(getInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.GetItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	nameAttr, _ = getOutput.Item["name"].(map[string]interface{})
	if nameAttr["S"] != "Alice Updated" {
		t.Errorf("expected updated name 'Alice Updated', got %v", nameAttr["S"])
	}

	// 6. Query
	queryInput := QueryInput{
		TableName: tableName,
		KeyConditionExpression: "pk = :pk",
		ExpressionAttributeValues: map[string]interface{}{
			":pk": map[string]interface{}{"S": "user#1"},
		},
	}
	bodyBytes, _ = json.Marshal(queryInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.Query")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	var queryOutput QueryOutput
	_ = json.NewDecoder(resp.Body).Decode(&queryOutput)
	if len(queryOutput.Items) != 1 {
		t.Errorf("expected 1 query result item, got %d", len(queryOutput.Items))
	}

	// 7. Delete Item
	delInput := DeleteItemInput{
		TableName: tableName,
		Key:       key,
	}
	bodyBytes, _ = json.Marshal(delInput)

	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.DeleteItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteItem status 200, got %d", resp.StatusCode)
	}

	// Verify Deleted
	bodyBytes, _ = json.Marshal(getInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.GetItem")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp = w.Result()
	getOutput = GetItemOutput{}
	_ = json.NewDecoder(resp.Body).Decode(&getOutput)
	if len(getOutput.Item) != 0 {
		t.Errorf("expected empty item after deletion, got %v", getOutput.Item)
	}
}
