package stepfunctions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
	"github.com/floci-io/floci-go/internal/services/lambda"
)

type StateMachine struct {
	Name            string    `json:"name"`
	StateMachineArn string    `json:"stateMachineArn"`
	Definition      string    `json:"definition"`
	RoleArn         string    `json:"roleArn"`
	CreationDate    float64   `json:"creationDate"`
}

type Execution struct {
	ExecutionArn    string  `json:"executionArn"`
	StateMachineArn string  `json:"stateMachineArn"`
	Name            string  `json:"name"`
	Status          string  `json:"status"` // RUNNING, SUCCEEDED, FAILED
	StartDate       float64 `json:"startDate"`
	StopDate        float64 `json:"stopDate,omitempty"`
	Input           string  `json:"input"`
	Output          string  `json:"output,omitempty"`
}

type StepFunctionsHandler struct {
	mu            sync.RWMutex
	machines      map[string]*StateMachine
	executions    map[string]*Execution
	lambdaHandler *lambda.LambdaHandler
	AccountID     string
}

func NewHandler(lambdaHandler *lambda.LambdaHandler) *StepFunctionsHandler {
	return &StepFunctionsHandler{
		machines:      make(map[string]*StateMachine),
		executions:    make(map[string]*Execution),
		lambdaHandler: lambdaHandler,
		AccountID:     "000000000000",
	}
}

func (h *StepFunctionsHandler) Name() string {
	return "stepfunctions"
}

func (h *StepFunctionsHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		return len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "stepfunctions")
	}
	return false
}

func (h *StepFunctionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	action := parts[1]

	switch action {
	case "CreateStateMachine":
		h.handleCreateStateMachine(w, r)
	case "DescribeStateMachine":
		h.handleDescribeStateMachine(w, r)
	case "StartExecution":
		h.handleStartExecution(w, r)
	case "DescribeExecution":
		h.handleDescribeExecution(w, r)
	case "StopExecution":
		h.handleStopExecution(w, r)
	case "ListStateMachines":
		h.handleListStateMachines(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action StepFunctions.%s not supported", action)).
			WriteJSONResponse(w, "AWSStepFunctions")
	}
}

func (h *StepFunctionsHandler) handleCreateStateMachine(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string `json:"name"`
		Definition string `json:"definition"`
		RoleArn    string `json:"roleArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	arn := fmt.Sprintf("arn:aws:states:us-east-1:%s:stateMachine:%s", h.AccountID, input.Name)
	if _, exists := h.machines[arn]; exists {
		awserr.New(400, "StateMachineAlreadyExists", "StateMachine already exists").WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	sm := &StateMachine{
		Name:            input.Name,
		StateMachineArn: arn,
		Definition:      input.Definition,
		RoleArn:         input.RoleArn,
		CreationDate:    float64(time.Now().Unix()),
	}

	h.machines[arn] = sm

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"stateMachineArn": sm.StateMachineArn,
		"creationDate":    sm.CreationDate,
	})
}

func (h *StepFunctionsHandler) handleDescribeStateMachine(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	sm, exists := h.machines[input.StateMachineArn]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "StateMachineDoesNotExist", "StateMachine does not exist").WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sm)
}

func (h *StepFunctionsHandler) handleStartExecution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StateMachineArn string `json:"stateMachineArn"`
		Name            string `json:"name,omitempty"`
		Input           string `json:"input,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	_, exists := h.machines[input.StateMachineArn]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "StateMachineDoesNotExist", "StateMachine does not exist").WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	execName := input.Name
	if execName == "" {
		execName = "exec-" + randomHex(4)
	}

	execArn := fmt.Sprintf("%s:execution:%s", input.StateMachineArn, execName)
	exInput := input.Input
	if exInput == "" {
		exInput = "{}"
	}

	ex := &Execution{
		ExecutionArn:    execArn,
		StateMachineArn: input.StateMachineArn,
		Name:            execName,
		Status:          "RUNNING",
		StartDate:       float64(time.Now().Unix()),
		Input:           exInput,
	}

	h.mu.Lock()
	h.executions[execArn] = ex
	h.mu.Unlock()

	// Trigger runner in background
	go h.runExecution(execArn)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"executionArn": ex.ExecutionArn,
		"startDate":    ex.StartDate,
	})
}

func (h *StepFunctionsHandler) handleDescribeExecution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExecutionArn string `json:"executionArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	ex, exists := h.executions[input.ExecutionArn]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "ExecutionDoesNotExist", "Execution does not exist").WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ex)
}

func (h *StepFunctionsHandler) handleStopExecution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExecutionArn string `json:"executionArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	ex, exists := h.executions[input.ExecutionArn]
	if exists && ex.Status == "RUNNING" {
		ex.Status = "ABORTED"
		ex.StopDate = float64(time.Now().Unix())
	}
	h.mu.Unlock()

	if !exists {
		awserr.New(404, "ExecutionDoesNotExist", "Execution does not exist").WriteJSONResponse(w, "AWSStepFunctions")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"stopDate": ex.StopDate})
}

func (h *StepFunctionsHandler) handleListStateMachines(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]*StateMachine, 0, len(h.machines))
	for _, sm := range h.machines {
		list = append(list, sm)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"stateMachines": list})
}

func (h *StepFunctionsHandler) runExecution(executionArn string) {
	h.mu.RLock()
	ex, exists := h.executions[executionArn]
	if !exists {
		h.mu.RUnlock()
		return
	}
	machine, exists := h.machines[ex.StateMachineArn]
	h.mu.RUnlock()

	if !exists {
		h.mu.Lock()
		ex.Status = "FAILED"
		ex.StopDate = float64(time.Now().Unix())
		ex.Output = `{"error": "StateMachineNotFound"}`
		h.mu.Unlock()
		return
	}

	var def struct {
		StartAt string                 `json:"StartAt"`
		States  map[string]interface{} `json:"States"`
	}
	if err := json.Unmarshal([]byte(machine.Definition), &def); err != nil {
		h.mu.Lock()
		ex.Status = "FAILED"
		ex.StopDate = float64(time.Now().Unix())
		errMsg, _ := json.Marshal(err.Error())
		ex.Output = fmt.Sprintf(`{"error": "InvalidASLDefinition", "message": %s}`, string(errMsg))
		h.mu.Unlock()
		return
	}

	currentStateName := def.StartAt
	currentInput := ex.Input
	steps := 0
	const maxSteps = 1000

	for {
		if currentStateName == "" {
			break
		}

		steps++
		if steps > maxSteps {
			currentInput = `{"error": "MaxStepsExceeded", "message": "Execution exceeded maximum step limit of 1000"}`
			break
		}

		h.mu.RLock()
		// Check for external stop execution
		if ex.Status == "ABORTED" {
			h.mu.RUnlock()
			return
		}
		h.mu.RUnlock()

		stateVal, ok := def.States[currentStateName]
		if !ok {
			stateMsg, _ := json.Marshal(currentStateName)
			currentInput = fmt.Sprintf(`{"error": "StateNotFound", "state": %s}`, string(stateMsg))
			break
		}

		state, ok := stateVal.(map[string]interface{})
		if !ok {
			currentInput = `{"error": "InvalidStateObject"}`
			break
		}

		stateType, _ := state["Type"].(string)
		var output string
		var err error

		switch stateType {
		case "Pass":
			if res, exists := state["Result"]; exists {
				resBytes, _ := json.Marshal(res)
				output = string(resBytes)
			} else {
				output = currentInput
			}
		case "Task":
			resource, _ := state["Resource"].(string)
			if strings.Contains(resource, "arn:aws:lambda") {
				parts := strings.Split(resource, ":")
				funcName := parts[len(parts)-1]

				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/2015-03-31/functions/%s/invocations", funcName), strings.NewReader(currentInput))
				h.lambdaHandler.ServeHTTP(rec, req)

				if rec.Code == http.StatusOK {
					output = rec.Body.String()
				} else {
					err = fmt.Errorf("lambda invocation failed with code %d: %s", rec.Code, rec.Body.String())
				}
			} else {
				output = currentInput
			}
		case "Succeed":
			output = currentInput
			currentStateName = ""
		case "Fail":
			output = currentInput
			currentStateName = ""
		default:
			output = currentInput
		}

		if err != nil {
			errMsg, _ := json.Marshal(err.Error())
			currentInput = fmt.Sprintf(`{"error": "ExecutionError", "message": %s}`, string(errMsg))
			break
		}

		currentInput = output

		isEnd, _ := state["End"].(bool)
		if isEnd || stateType == "Succeed" || stateType == "Fail" || currentStateName == "" {
			break
		}

		nextState, _ := state["Next"].(string)
		currentStateName = nextState
	}

	h.mu.Lock()
	if ex.Status != "ABORTED" {
		ex.Status = "SUCCEEDED"
		if strings.Contains(currentInput, `"error":`) {
			ex.Status = "FAILED"
		}
		ex.StopDate = float64(time.Now().Unix())
		ex.Output = currentInput
	}
	h.mu.Unlock()
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (h *StepFunctionsHandler) GetStateMachines() []*StateMachine {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*StateMachine, 0, len(h.machines))
	for _, sm := range h.machines {
		list = append(list, sm)
	}
	return list
}

func (h *StepFunctionsHandler) GetExecutions() []*Execution {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]*Execution, 0, len(h.executions))
	for _, ex := range h.executions {
		list = append(list, ex)
	}
	return list
}
