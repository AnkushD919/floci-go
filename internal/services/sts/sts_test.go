package sts

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSTSGetCallerIdentity(t *testing.T) {
	handler := NewHandler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action": []string{"GetCallerIdentity"},
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "/xml") {
		t.Errorf("expected Content-Type containing xml, got %s", contentType)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<UserId>000000000000</UserId>") {
		t.Errorf("expected body to contain UserId, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "<Account>000000000000</Account>") {
		t.Errorf("expected body to contain Account, got %s", bodyStr)
	}
}

func TestSTSAssumeRole(t *testing.T) {
	handler := NewHandler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action":           []string{"AssumeRole"},
		"RoleArn":          []string{"arn:aws:iam::000000000000:role/my-role-name"},
		"RoleSessionName": []string{"test-session"},
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<AccessKeyId>ASIA") {
		t.Errorf("expected body to contain AccessKeyId starting with ASIA, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "<Arn>arn:aws:sts::000000000000:assumed-role/my-role-name/test-session</Arn>") {
		t.Errorf("expected body to contain assumed role ARN, got %s", bodyStr)
	}
}

func TestSTSGetSessionToken(t *testing.T) {
	handler := NewHandler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action": []string{"GetSessionToken"},
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<AccessKeyId>ASIA") {
		t.Errorf("expected body to contain AccessKeyId starting with ASIA, got %s", bodyStr)
	}
}
