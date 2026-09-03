package sts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	stsNamespace = "https://sts.amazonaws.com/doc/2011-06-15/"
	defaultRegion = "us-east-1"
)

type STSHandler struct {
	AccountID string
}

func (h *STSHandler) Name() string {
	return "sts"
}

func (h *STSHandler) Matches(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	if strings.Contains(host, "sts.") {
		return true
	}
	action := r.URL.Query().Get("Action")
	if action == "" && strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}
	return action == "GetCallerIdentity" || action == "AssumeRole" || action == "GetSessionToken"
}

func NewHandler() *STSHandler {
	return &STSHandler{
		AccountID: "000000000000",
	}
}

type ResponseMetadata struct {
	RequestID string `xml:"RequestId"`
}

type GetCallerIdentityResult struct {
	UserID  string `xml:"UserId"`
	Account string `xml:"Account"`
	Arn     string `xml:"Arn"`
}

type GetCallerIdentityResponse struct {
	XMLName          xml.Name                `xml:"GetCallerIdentityResponse"`
	Xmlns            string                  `xml:"xmlns,attr"`
	Result           GetCallerIdentityResult `xml:"GetCallerIdentityResult"`
	ResponseMetadata ResponseMetadata        `xml:"ResponseMetadata"`
}

type Credentials struct {
	AccessKeyID     string `xml:"AccessKeyId"`
	SecretAccessKey string `xml:"SecretAccessKey"`
	SessionToken    string `xml:"SessionToken"`
	Expiration      string `xml:"Expiration"`
}

type AssumedRoleUser struct {
	Arn           string `xml:"Arn"`
	AssumedRoleID string `xml:"AssumedRoleId"`
}

type AssumeRoleResult struct {
	Credentials      Credentials     `xml:"Credentials"`
	AssumedRoleUser  AssumedRoleUser `xml:"AssumedRoleUser"`
	PackedPolicySize int             `xml:"PackedPolicySize"`
}

type AssumeRoleResponse struct {
	XMLName          xml.Name         `xml:"AssumeRoleResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	Result           AssumeRoleResult `xml:"AssumeRoleResult"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

type GetSessionTokenResult struct {
	Credentials Credentials `xml:"Credentials"`
}

type GetSessionTokenResponse struct {
	XMLName          xml.Name              `xml:"GetSessionTokenResponse"`
	Xmlns            string                `xml:"xmlns,attr"`
	Result           GetSessionTokenResult `xml:"GetSessionTokenResult"`
	ResponseMetadata ResponseMetadata      `xml:"ResponseMetadata"`
}

func (h *STSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("Action")
	if action == "" {
		_ = r.ParseForm()
		action = r.FormValue("Action")
	}

	switch action {
	case "GetCallerIdentity":
		h.handleGetCallerIdentity(w, r)
	case "AssumeRole":
		h.handleAssumeRole(w, r)
	case "GetSessionToken":
		h.handleGetSessionToken(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("The action %s is not supported.", action)).
			WriteXMLResponse(w, stsNamespace)
	}
}

func (h *STSHandler) handleGetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	resp := GetCallerIdentityResponse{
		Xmlns: stsNamespace,
		Result: GetCallerIdentityResult{
			UserID:  h.AccountID,
			Account: h.AccountID,
			Arn:     fmt.Sprintf("arn:aws:iam::%s:root", h.AccountID),
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func (h *STSHandler) handleAssumeRole(w http.ResponseWriter, r *http.Request) {
	roleArn := r.FormValue("RoleArn")
	sessionName := r.FormValue("RoleSessionName")

	if roleArn == "" || sessionName == "" {
		awserr.New(400, "MissingParameter", "RoleArn and RoleSessionName are required parameters.").
			WriteXMLResponse(w, stsNamespace)
		return
	}

	expiration := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	accessKey := "ASIA" + randomHex(8)
	secretKey := randomHex(20)
	token := randomHex(60)

	resp := AssumeRoleResponse{
		Xmlns: stsNamespace,
		Result: AssumeRoleResult{
			Credentials: Credentials{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
				SessionToken:    token,
				Expiration:      expiration,
			},
			AssumedRoleUser: AssumedRoleUser{
				Arn:           fmt.Sprintf("arn:aws:sts::%s:assumed-role/%s/%s", h.AccountID, extractRoleName(roleArn), sessionName),
				AssumedRoleID: "AROA" + randomHex(8) + ":" + sessionName,
			},
			PackedPolicySize: 0,
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: "00000000-0000-0000-0000-000000000000",
		},
	}

	writeXML(w, resp)
}

func (h *STSHandler) handleGetSessionToken(w http.ResponseWriter, r *http.Request) {
	expiration := time.Now().Add(12 * time.Hour).UTC().Format(time.RFC3339)
	accessKey := "ASIA" + randomHex(8)
	secretKey := randomHex(20)
	token := randomHex(60)

	resp := GetSessionTokenResponse{
		Xmlns: stsNamespace,
		Result: GetSessionTokenResult{
			Credentials: Credentials{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
				SessionToken:    token,
				Expiration:      expiration,
			},
		},
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

func extractRoleName(roleArn string) string {
	// Simple extractor: arn:aws:iam::123456789012:role/my-role -> my-role
	for i := len(roleArn) - 1; i >= 0; i-- {
		if roleArn[i] == '/' {
			return roleArn[i+1:]
		}
	}
	return "mock-role"
}
