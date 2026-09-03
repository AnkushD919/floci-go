package iam

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	iamNamespace = "https://iam.amazonaws.com/doc/2010-05-08/"
)

type Role struct {
	Path                     string `xml:"Path"`
	RoleName                 string `xml:"RoleName"`
	RoleID                   string `xml:"RoleId"`
	Arn                      string `xml:"Arn"`
	CreateDate               string `xml:"CreateDate"`
	AssumeRolePolicyDocument string `xml:"AssumeRolePolicyDocument"`
}

type IAMHandler struct {
	mu        sync.RWMutex
	roles     map[string]Role
	AccountID string
}

func (h *IAMHandler) Name() string {
	return "iam"
}

func (h *IAMHandler) Matches(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	if strings.Contains(host, "iam.") {
		return true
	}
	action := r.URL.Query().Get("Action")
	if action == "" && strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}
	return action != "" && (strings.Contains(action, "User") || strings.Contains(action, "Role") || strings.Contains(action, "Policy"))
}

func NewHandler() *IAMHandler {
	return &IAMHandler{
		roles:     make(map[string]Role),
		AccountID: "000000000000",
	}
}

type ResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

type CreateRoleResult struct {
	Role Role `xml:"Role"`
}

type CreateRoleResponse struct {
	XMLName          xml.Name         `xml:"CreateRoleResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	Result           CreateRoleResult `xml:"CreateRoleResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type GetRoleResult struct {
	Role Role `xml:"Role"`
}

type GetRoleResponse struct {
	XMLName          xml.Name         `xml:"GetRoleResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	Result           GetRoleResult    `xml:"GetRoleResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type ListRolesResult struct {
	Roles       []Role `xml:"Roles>member"`
	IsTruncated bool   `xml:"IsTruncated"`
}

type ListRolesResponse struct {
	XMLName          xml.Name        `xml:"ListRolesResponse"`
	Xmlns            string          `xml:"xmlns,attr"`
	Result           ListRolesResult `xml:"ListRolesResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type DeleteRoleResponse struct {
	XMLName          xml.Name         `xml:"DeleteRoleResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

func (h *IAMHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("Action")
	if action == "" {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}

	switch action {
	case "CreateRole":
		h.handleCreateRole(w, r)
	case "GetRole":
		h.handleGetRole(w, r)
	case "ListRoles":
		h.handleListRoles(w, r)
	case "DeleteRole":
		h.handleDeleteRole(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteXMLResponse(w, iamNamespace)
	}
}

func (h *IAMHandler) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	roleName := r.FormValue("RoleName")
	assumePolicy := r.FormValue("AssumeRolePolicyDocument")
	path := r.FormValue("Path")

	if roleName == "" || assumePolicy == "" {
		awserr.New(400, "MissingParameter", "RoleName and AssumeRolePolicyDocument are required parameters.").
			WriteXMLResponse(w, iamNamespace)
		return
	}

	if path == "" {
		path = "/"
	}

	// URL-encode the assume role policy document as expected by AWS
	encodedPolicy := url.QueryEscape(assumePolicy)

	role := Role{
		Path:                     path,
		RoleName:                 roleName,
		RoleID:                   "AROA" + randomHex(8),
		Arn:                      fmt.Sprintf("arn:aws:iam::%s:role/%s", h.AccountID, roleName),
		CreateDate:               time.Now().UTC().Format(time.RFC3339),
		AssumeRolePolicyDocument: encodedPolicy,
	}

	h.roles[roleName] = role

	resp := CreateRoleResponse{
		Xmlns: iamNamespace,
		Result: CreateRoleResult{
			Role: role,
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func (h *IAMHandler) handleGetRole(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	roleName := r.FormValue("RoleName")
	if roleName == "" {
		awserr.New(400, "MissingParameter", "RoleName is a required parameter.").
			WriteXMLResponse(w, iamNamespace)
		return
	}

	role, exists := h.roles[roleName]
	if !exists {
		awserr.New(404, "NoSuchEntity", fmt.Sprintf("The role with name %s cannot be found.", roleName)).
			WriteXMLResponse(w, iamNamespace)
		return
	}

	resp := GetRoleResponse{
		Xmlns: iamNamespace,
		Result: GetRoleResult{
			Role: role,
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func (h *IAMHandler) handleListRoles(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	rolesList := make([]Role, 0, len(h.roles))
	for _, role := range h.roles {
		rolesList = append(rolesList, role)
	}

	resp := ListRolesResponse{
		Xmlns: iamNamespace,
		Result: ListRolesResult{
			Roles:       rolesList,
			IsTruncated: false,
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func (h *IAMHandler) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	roleName := r.FormValue("RoleName")
	if roleName == "" {
		awserr.New(400, "MissingParameter", "RoleName is a required parameter.").
			WriteXMLResponse(w, iamNamespace)
		return
	}

	_, exists := h.roles[roleName]
	if !exists {
		awserr.New(404, "NoSuchEntity", fmt.Sprintf("The role with name %s cannot be found.", roleName)).
			WriteXMLResponse(w, iamNamespace)
		return
	}

	delete(h.roles, roleName)

	resp := DeleteRoleResponse{
		Xmlns: iamNamespace,
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func writeXML(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(data)
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "mockedrandombytesstring"
	}
	return hex.EncodeToString(bytes)
}

func (h *IAMHandler) GetRoles() []Role {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]Role, 0, len(h.roles))
	for _, r := range h.roles {
		res = append(res, r)
	}
	return res
}
