package dynamodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	ddbTargetPrefix = "DynamoDB_20120810"
)

type AttributeDefinition struct {
	AttributeName string `json:"AttributeName"`
	AttributeType string `json:"AttributeType"`
}

type KeySchemaElement struct {
	AttributeName string `json:"AttributeName"`
	KeyType       string `json:"KeyType"` // HASH or RANGE
}

type TableDescription struct {
	TableName            string                `json:"TableName"`
	TableStatus          string                `json:"TableStatus"`
	AttributeDefinitions []AttributeDefinition `json:"AttributeDefinitions"`
	KeySchema            []KeySchemaElement    `json:"KeySchema"`
	ItemCount            int64                 `json:"ItemCount"`
	TableSizeBytes       int64                 `json:"TableSizeBytes"`
}

type CreateTableInput struct {
	TableName            string                `json:"TableName"`
	AttributeDefinitions []AttributeDefinition `json:"AttributeDefinitions"`
	KeySchema            []KeySchemaElement    `json:"KeySchema"`
	BillingMode          string                `json:"BillingMode"`
}

type CreateTableOutput struct {
	TableDescription TableDescription `json:"TableDescription"`
}

type DescribeTableInput struct {
	TableName string `json:"TableName"`
}

type DescribeTableOutput struct {
	Table TableDescription `json:"Table"`
}

type ListTablesInput struct {
	Limit             int    `json:"Limit"`
	ExclusiveStartTableName string `json:"ExclusiveStartTableName"`
}

type ListTablesOutput struct {
	TableNames []string `json:"TableNames"`
}

type PutItemInput struct {
	TableName string                 `json:"TableName"`
	Item      map[string]interface{} `json:"Item"`
}

type GetItemInput struct {
	TableName string                 `json:"TableName"`
	Key       map[string]interface{} `json:"Key"`
}

type GetItemOutput struct {
	Item map[string]interface{} `json:"Item,omitempty"`
}

type DeleteItemInput struct {
	TableName string                 `json:"TableName"`
	Key       map[string]interface{} `json:"Key"`
}

type DeleteTableInput struct {
	TableName string `json:"TableName"`
}

type ScanInput struct {
	TableName string `json:"TableName"`
}

type ScanOutput struct {
	Items []map[string]interface{} `json:"Items"`
	Count int                      `json:"Count"`
}

type QueryInput struct {
	TableName                 string                 `json:"TableName"`
	KeyConditionExpression    string                 `json:"KeyConditionExpression"`
	ExpressionAttributeNames  map[string]string      `json:"ExpressionAttributeNames"`
	ExpressionAttributeValues map[string]interface{} `json:"ExpressionAttributeValues"`
}

type QueryOutput struct {
	Items []map[string]interface{} `json:"Items"`
	Count int                      `json:"Count"`
}

type UpdateItemInput struct {
	TableName                 string                 `json:"TableName"`
	Key                       map[string]interface{} `json:"Key"`
	UpdateExpression          string                 `json:"UpdateExpression"`
	ExpressionAttributeNames  map[string]string      `json:"ExpressionAttributeNames"`
	ExpressionAttributeValues map[string]interface{} `json:"ExpressionAttributeValues"`
}

type Table struct {
	Meta  TableDescription
	Items []map[string]interface{}
}

type DynamoDBHandler struct {
	mu        sync.RWMutex
	tables    map[string]*Table
	AccountID string
}

func (h *DynamoDBHandler) Name() string {
	return "dynamodb"
}

func (h *DynamoDBHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		if len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "dynamodb") {
			return true
		}
	}
	return false
}

func NewHandler() *DynamoDBHandler {
	return &DynamoDBHandler{
		tables:    make(map[string]*Table),
		AccountID: "000000000000",
	}
}

func (h *DynamoDBHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	action := parts[1]

	switch action {
	case "CreateTable":
		h.handleCreateTable(w, r)
	case "DescribeTable":
		h.handleDescribeTable(w, r)
	case "ListTables":
		h.handleListTables(w, r)
	case "PutItem":
		h.handlePutItem(w, r)
	case "GetItem":
		h.handleGetItem(w, r)
	case "UpdateItem":
		h.handleUpdateItem(w, r)
	case "DeleteItem":
		h.handleDeleteItem(w, r)
	case "DeleteTable":
		h.handleDeleteTable(w, r)
	case "Scan":
		h.handleScan(w, r)
	case "Query":
		h.handleQuery(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteJSONResponse(w, ddbTargetPrefix)
	}
}

func (h *DynamoDBHandler) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input CreateTableInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	if _, exists := h.tables[input.TableName]; exists {
		awserr.New(400, "ResourceInUseException", fmt.Sprintf("Table already exists: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	desc := TableDescription{
		TableName:            input.TableName,
		TableStatus:          "ACTIVE",
		AttributeDefinitions: input.AttributeDefinitions,
		KeySchema:            input.KeySchema,
		ItemCount:            0,
		TableSizeBytes:       0,
	}

	h.tables[input.TableName] = &Table{
		Meta:  desc,
		Items: make([]map[string]interface{}, 0),
	}

	writeJSON(w, CreateTableOutput{TableDescription: desc})
}

func (h *DynamoDBHandler) handleDescribeTable(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input DescribeTableInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	writeJSON(w, DescribeTableOutput{Table: table.Meta})
}

func (h *DynamoDBHandler) handleListTables(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	names := make([]string, 0, len(h.tables))
	for name := range h.tables {
		names = append(names, name)
	}

	writeJSON(w, ListTablesOutput{TableNames: names})
}

func (h *DynamoDBHandler) handlePutItem(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input PutItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	// Find and replace or append
	idx := h.findItemIndex(table, input.Item)
	if idx != -1 {
		table.Items[idx] = input.Item
	} else {
		table.Items = append(table.Items, input.Item)
	}

	table.Meta.ItemCount = int64(len(table.Items))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (h *DynamoDBHandler) handleGetItem(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input GetItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	idx := h.findItemIndex(table, input.Key)
	if idx == -1 {
		writeJSON(w, GetItemOutput{})
		return
	}

	writeJSON(w, GetItemOutput{Item: table.Items[idx]})
}

func (h *DynamoDBHandler) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input DeleteItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	idx := h.findItemIndex(table, input.Key)
	if idx != -1 {
		table.Items = append(table.Items[:idx], table.Items[idx+1:]...)
	}

	table.Meta.ItemCount = int64(len(table.Items))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (h *DynamoDBHandler) handleDeleteTable(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input DeleteTableInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	if _, exists := h.tables[input.TableName]; !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	delete(h.tables, input.TableName)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (h *DynamoDBHandler) handleScan(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input ScanInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	writeJSON(w, ScanOutput{
		Items: table.Items,
		Count: len(table.Items),
	})
}

func (h *DynamoDBHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var input QueryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	// Pragmatic Query implementation:
	// Find the Hash Key attribute name from KeySchema
	var hashKeyName string
	for _, k := range table.Meta.KeySchema {
		if k.KeyType == "HASH" {
			hashKeyName = k.AttributeName
			break
		}
	}

	if hashKeyName == "" {
		writeJSON(w, QueryOutput{Items: table.Items, Count: len(table.Items)})
		return
	}

	// Resolve the query value for the Hash Key from ExpressionAttributeValues
	var hashValue interface{}
	// Usually KeyConditionExpression contains "pk = :pk" or similar
	for placeholder, valObj := range input.ExpressionAttributeValues {
		if strings.Contains(input.KeyConditionExpression, placeholder) {
			hashValue = valObj
			break
		}
	}

	results := make([]map[string]interface{}, 0)
	for _, item := range table.Items {
		itemVal, ok := item[hashKeyName]
		if ok && matchAttributeValue(itemVal, hashValue) {
			results = append(results, item)
		}
	}

	writeJSON(w, QueryOutput{
		Items: results,
		Count: len(results),
	})
}

func (h *DynamoDBHandler) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var input UpdateItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		awserr.New(400, "SerializationException", err.Error()).WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	table, exists := h.tables[input.TableName]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", fmt.Sprintf("Table not found: %s", input.TableName)).
			WriteJSONResponse(w, ddbTargetPrefix)
		return
	}

	idx := h.findItemIndex(table, input.Key)
	var item map[string]interface{}
	if idx != -1 {
		item = table.Items[idx]
	} else {
		// Create new item with keys
		item = make(map[string]interface{})
		for k, v := range input.Key {
			item[k] = v
		}
		table.Items = append(table.Items, item)
		idx = len(table.Items) - 1
	}

	// Evaluate UpdateExpression: "SET #n = :n, age = :a"
	updateExpr := input.UpdateExpression
	if strings.HasPrefix(updateExpr, "SET ") {
		clause := strings.TrimPrefix(updateExpr, "SET ")
		assignments := strings.Split(clause, ",")
		for _, assoc := range assignments {
			parts := strings.Split(assoc, "=")
			if len(parts) == 2 {
				lhs := strings.TrimSpace(parts[0])
				rhs := strings.TrimSpace(parts[1])

				// Resolve LHS using ExpressionAttributeNames
				attrName := lhs
				if strings.HasPrefix(lhs, "#") {
					if resolved, ok := input.ExpressionAttributeNames[lhs]; ok {
						attrName = resolved
					}
				}

				// Resolve RHS using ExpressionAttributeValues
				var attrVal interface{}
				if strings.HasPrefix(rhs, ":") {
					if resolved, ok := input.ExpressionAttributeValues[rhs]; ok {
						attrVal = resolved
					}
				}

				item[attrName] = attrVal
			}
		}
	}

	table.Items[idx] = item

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (h *DynamoDBHandler) findItemIndex(table *Table, key map[string]interface{}) int {
	for idx, item := range table.Items {
		matched := true
		for _, kSchema := range table.Meta.KeySchema {
			kName := kSchema.AttributeName
			keyVal, keyExists := key[kName]
			itemVal, itemExists := item[kName]

			if !keyExists {
				continue
			}

			if !itemExists || !matchAttributeValue(itemVal, keyVal) {
				matched = false
				break
			}
		}
		if matched {
			return idx
		}
	}
	return -1
}

func matchAttributeValue(val1 interface{}, val2 interface{}) bool {
	// Attribute values in JSON look like: {"S": "value"} or {"N": "30"}
	m1, ok1 := val1.(map[string]interface{})
	m2, ok2 := val2.(map[string]interface{})

	if !ok1 || !ok2 {
		return false
	}

	for k, v1 := range m1 {
		v2, ok := m2[k]
		if ok && fmt.Sprintf("%v", v1) == fmt.Sprintf("%v", v2) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *DynamoDBHandler) GetTables() []*Table {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]*Table, 0, len(h.tables))
	for _, t := range h.tables {
		res = append(res, t)
	}
	return res
}
