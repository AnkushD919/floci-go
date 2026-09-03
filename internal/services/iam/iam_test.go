package iam

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIAMRoleLifecycle(t *testing.T) {
	handler := NewHandler()

	roleName := "test-role-name"
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

	// 1. Create Role
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action":                   []string{"CreateRole"},
		"RoleName":                 []string{roleName},
		"AssumeRolePolicyDocument": []string{policy},
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected CreateRole status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<RoleName>test-role-name</RoleName>") {
		t.Errorf("expected body to contain RoleName, got %s", bodyStr)
	}

	// 2. Get Role
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action":   []string{"GetRole"},
		"RoleName": []string{roleName},
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected GetRole status 200, got %d", resp.StatusCode)
	}

	body, _ = io.ReadAll(resp.Body)
	bodyStr = string(body)
	if !strings.Contains(bodyStr, "<RoleName>test-role-name</RoleName>") {
		t.Errorf("expected GetRole response to contain RoleName, got %s", bodyStr)
	}

	// 3. List Roles
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action": []string{"ListRoles"},
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	body, _ = io.ReadAll(resp.Body)
	bodyStr = string(body)
	if !strings.Contains(bodyStr, "<RoleName>test-role-name</RoleName>") {
		t.Errorf("expected ListRoles to contain our role, got %s", bodyStr)
	}

	// 4. Delete Role
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action":   []string{"DeleteRole"},
		"RoleName": []string{roleName},
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected DeleteRole status 200, got %d", resp.StatusCode)
	}

	// 5. Verify Deleted (Get Role should now return 404)
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.PostForm = url.Values{
		"Action":   []string{"GetRole"},
		"RoleName": []string{roleName},
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected GetRole after deletion to return 404, got %d", resp.StatusCode)
	}
}
