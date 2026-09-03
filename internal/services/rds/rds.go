package rds

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/floci-io/floci-go/internal/awserr"
	_ "modernc.org/sqlite"
)

type DBInstance struct {
	DBInstanceIdentifier string `json:"DBInstanceIdentifier"`
	DBInstanceStatus     string `json:"DBInstanceStatus"`
	Engine               string `json:"Engine"`
	Address              string `json:"Address"`
	DBName               string `json:"DBName,omitempty"`
}

type SqlParameterValue struct {
	StringValue  *string  `json:"stringValue,omitempty"`
	DoubleValue  *float64 `json:"doubleValue,omitempty"`
	LongValue    *int64   `json:"longValue,omitempty"`
	BooleanValue *bool    `json:"booleanValue,omitempty"`
	IsNull       *bool    `json:"isNull,omitempty"`
}

type SqlParameter struct {
	Name  string             `json:"name"`
	Value SqlParameterValue `json:"value"`
}

type Field struct {
	StringValue  *string  `json:"stringValue,omitempty"`
	DoubleValue  *float64 `json:"doubleValue,omitempty"`
	LongValue    *int64   `json:"longValue,omitempty"`
	BooleanValue *bool    `json:"booleanValue,omitempty"`
	IsNull       *bool    `json:"isNull,omitempty"`
}

type ColumnMetadata struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ExecuteStatementInput struct {
	SecretArn   string         `json:"secretArn"`
	ResourceArn string         `json:"resourceArn"`
	Sql         string         `json:"sql"`
	Database    string         `json:"database"`
	Parameters  []SqlParameter `json:"parameters,omitempty"`
}

type ExecuteStatementOutput struct {
	Records        [][]Field        `json:"records"`
	ColumnMetadata []ColumnMetadata `json:"columnMetadata"`
}

type RDSHandler struct {
	mu        sync.RWMutex
	instances map[string]*DBInstance
	dbs       map[string]*sql.DB // databaseName -> *sql.DB
	AccountID string
}

func NewHandler() *RDSHandler {
	return &RDSHandler{
		instances: make(map[string]*DBInstance),
		dbs:       make(map[string]*sql.DB),
		AccountID: "000000000000",
	}
}

func (h *RDSHandler) Name() string {
	return "rds"
}

func (h *RDSHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		return len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "rds")
	}
	return false
}

func (h *RDSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, "RDS")
		return
	}

	prefix := parts[0]
	action := parts[1]

	if prefix == "AmazonRDSv12" {
		h.handleControlPlane(w, r, action)
	} else if prefix == "RDSDataService" {
		h.handleDataAPI(w, r, action)
	} else {
		awserr.New(400, "InvalidAction", fmt.Sprintf("Prefix %s not supported", prefix)).
			WriteJSONResponse(w, "RDS")
	}
}

func (h *RDSHandler) handleControlPlane(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "CreateDBInstance":
		h.handleCreateDBInstance(w, r)
	case "DescribeDBInstances":
		h.handleDescribeDBInstances(w, r)
	case "DeleteDBInstance":
		h.handleDeleteDBInstance(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action RDS.%s not supported", action)).
			WriteJSONResponse(w, "AmazonRDSv12")
	}
}

func (h *RDSHandler) handleDataAPI(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "ExecuteStatement":
		h.handleExecuteStatement(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action RDSData.%s not supported", action)).
			WriteJSONResponse(w, "RDSDataService")
	}
}

func (h *RDSHandler) handleCreateDBInstance(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DBInstanceIdentifier string `json:"DBInstanceIdentifier"`
		Engine               string `json:"Engine"`
		DBName               string `json:"DBName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.instances[input.DBInstanceIdentifier]; exists {
		awserr.New(400, "DBInstanceAlreadyExists", "DB instance already exists").WriteJSONResponse(w, "AmazonRDSv12")
		return
	}

	inst := &DBInstance{
		DBInstanceIdentifier: input.DBInstanceIdentifier,
		DBInstanceStatus:     "available",
		Engine:               input.Engine,
		Address:              fmt.Sprintf("%s.rds.localhost", input.DBInstanceIdentifier),
		DBName:               input.DBName,
	}

	h.instances[input.DBInstanceIdentifier] = inst

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]*DBInstance{"DBInstance": inst})
}

func (h *RDSHandler) handleDescribeDBInstances(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*DBInstance, 0, len(h.instances))
	for _, inst := range h.instances {
		list = append(list, inst)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"DBInstances": list})
}

func (h *RDSHandler) handleDeleteDBInstance(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DBInstanceIdentifier string `json:"DBInstanceIdentifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	inst, exists := h.instances[input.DBInstanceIdentifier]
	if !exists {
		awserr.New(404, "DBInstanceNotFound", "DB instance not found").WriteJSONResponse(w, "AmazonRDSv12")
		return
	}

	// Close open SQLite database connection for this instance
	dbName := inst.DBName
	if dbName != "" {
		if db, exists := h.dbs[dbName]; exists {
			db.Close()
			delete(h.dbs, dbName)
			// Remove the local SQLite file
			dbPath := filepath.Join(os.TempDir(), "floci-rds", dbName+".db")
			_ = os.Remove(dbPath)
		}
	}

	delete(h.instances, input.DBInstanceIdentifier)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]*DBInstance{"DBInstance": inst})
}

func (h *RDSHandler) handleExecuteStatement(w http.ResponseWriter, r *http.Request) {
	var input ExecuteStatementInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dbName := input.Database
	if dbName == "" {
		// Fallback to name parsed from ResourceArn
		parts := strings.Split(input.ResourceArn, ":")
		if len(parts) > 0 {
			dbName = parts[len(parts)-1]
		}
	}
	if dbName == "" {
		dbName = "default"
	}

	h.mu.Lock()
	db, exists := h.dbs[dbName]
	if !exists {
		dbDir := filepath.Join(os.TempDir(), "floci-rds")
		if err := os.MkdirAll(dbDir, 0o750); err != nil {
			h.mu.Unlock()
			http.Error(w, fmt.Sprintf("Failed to create database directory: %v", err), http.StatusInternalServerError)
			return
		}
		dbPath := filepath.Join(dbDir, dbName+".db")
		var err error
		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			h.mu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.dbs[dbName] = db
	}
	h.mu.Unlock()

	// Parse parameters
	args := make([]interface{}, 0, len(input.Parameters))
	for _, p := range input.Parameters {
		var val interface{}
		if p.Value.StringValue != nil {
			val = *p.Value.StringValue
		} else if p.Value.LongValue != nil {
			val = *p.Value.LongValue
		} else if p.Value.DoubleValue != nil {
			val = *p.Value.DoubleValue
		} else if p.Value.BooleanValue != nil {
			val = *p.Value.BooleanValue
		} else if p.Value.IsNull != nil && *p.Value.IsNull {
			val = nil
		}
		// Bind standard named argument (driver automatically resolves :name or @name)
		args = append(args, sql.Named(p.Name, val))
	}

	// Execute statement
	// SQLite supports multiple commands via transaction/statements, but sql.Query handles standard SELECT/INSERT/CREATE/UPDATE
	// If query is writing (INSERT, UPDATE, DELETE, CREATE), we can execute it via Exec and return empty records.
	// Otherwise, we execute via Query and return columns + records.
	sqlUpper := strings.ToUpper(strings.TrimSpace(input.Sql))
	isWrite := strings.HasPrefix(sqlUpper, "INSERT") ||
		strings.HasPrefix(sqlUpper, "UPDATE") ||
		strings.HasPrefix(sqlUpper, "DELETE") ||
		strings.HasPrefix(sqlUpper, "CREATE") ||
		strings.HasPrefix(sqlUpper, "DROP") ||
		strings.HasPrefix(sqlUpper, "ALTER")

	if isWrite {
		_, err := db.Exec(input.Sql, args...)
		if err != nil {
			awserr.New(400, "BadRequestException", err.Error()).WriteJSONResponse(w, "RDSDataService")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ExecuteStatementOutput{
			Records:        [][]Field{},
			ColumnMetadata: []ColumnMetadata{},
		})
		return
	}

	// Read query (SELECT or PRAGMA)
	rows, err := db.Query(input.Sql, args...)
	if err != nil {
		awserr.New(400, "BadRequestException", err.Error()).WriteJSONResponse(w, "RDSDataService")
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	metadata := make([]ColumnMetadata, len(cols))
	for i, col := range cols {
		metadata[i] = ColumnMetadata{Name: col, Type: "VARCHAR"} // SQLite type is dynamic, default VARCHAR
	}

	records := make([][]Field, 0)
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		valPtrs := make([]interface{}, len(cols))
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		if err := rows.Scan(valPtrs...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rowFields := make([]Field, len(cols))
		for i, val := range vals {
			field := Field{}
			if val == nil {
				field.IsNull = boolPtr(true)
			} else {
				switch v := val.(type) {
				case string:
					field.StringValue = &v
				case int64:
					field.LongValue = &v
				case float64:
					field.DoubleValue = &v
				case bool:
					field.BooleanValue = &v
				case []byte:
					s := string(v)
					field.StringValue = &s
				default:
					s := fmt.Sprintf("%v", v)
					field.StringValue = &s
				}
			}
			rowFields[i] = field
		}
		records = append(records, rowFields)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ExecuteStatementOutput{
		Records:        records,
		ColumnMetadata: metadata,
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func (h *RDSHandler) GetInstances() []*DBInstance {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*DBInstance, 0, len(h.instances))
	for _, inst := range h.instances {
		list = append(list, inst)
	}
	return list
}
