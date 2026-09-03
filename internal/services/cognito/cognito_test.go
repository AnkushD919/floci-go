package cognito

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCognitoLifecycle(t *testing.T) {
	handler := NewHandler()

	// 1. CreateUserPool
	createPoolInput := map[string]string{
		"PoolName": "test-pool",
	}
	bodyBytes, _ := json.Marshal(createPoolInput)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.CreateUserPool")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected CreateUserPool status 200, got %d", resp.StatusCode)
	}

	var createPoolOutput map[string]map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&createPoolOutput)
	poolId, _ := createPoolOutput["UserPool"]["Id"].(string)
	if poolId == "" {
		t.Fatalf("expected userPoolId in output")
	}

	// 2. AdminCreateUser
	createUserInput := map[string]interface{}{
		"UserPoolId": poolId,
		"Username":   "test-user",
		"UserAttributes": []map[string]string{
			{"Name": "email", "Value": "test@example.com"},
		},
	}
	bodyBytes, _ = json.Marshal(createUserInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.AdminCreateUser")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected AdminCreateUser status 200, got %d", resp.StatusCode)
	}

	// 3. AdminSetUserPassword
	setPasswordInput := map[string]interface{}{
		"UserPoolId": poolId,
		"Username":   "test-user",
		"Password":   "SecurePassword123!",
	}
	bodyBytes, _ = json.Marshal(setPasswordInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.AdminSetUserPassword")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected AdminSetUserPassword status 200, got %d", resp.StatusCode)
	}

	// 4. InitiateAuth (Login)
	authInput := map[string]interface{}{
		"AuthFlow": "USER_PASSWORD_AUTH",
		"ClientId": "client-12345",
		"AuthParameters": map[string]string{
			"USERNAME": "test-user",
			"PASSWORD": "SecurePassword123!",
		},
	}
	bodyBytes, _ = json.Marshal(authInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.InitiateAuth")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected InitiateAuth status 200, got %d", resp.StatusCode)
	}

	var authOutput map[string]map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&authOutput)
	accessToken, _ := authOutput["AuthenticationResult"]["AccessToken"].(string)
	idToken, _ := authOutput["AuthenticationResult"]["IdToken"].(string)
	if accessToken == "" || idToken == "" {
		t.Fatalf("expected JWT AccessToken and IdToken in AuthenticationResult")
	}

	// 5. GetUser (Retrieve user attributes using AccessToken)
	getUserInput := map[string]interface{}{
		"AccessToken": accessToken,
	}
	bodyBytes, _ = json.Marshal(getUserInput)
	req = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AWSCognitoIdentityProviderService.GetUser")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GetUser status 200, got %d", resp.StatusCode)
	}

	var getUserOutput map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&getUserOutput)
	username, _ := getUserOutput["Username"].(string)
	if username != "test-user" {
		t.Errorf("expected Username 'test-user', got '%s'", username)
	}

	// Verify token claim decoding locally
	claims, err := verifyJWT(accessToken, jwtSecret)
	if err != nil {
		t.Fatalf("failed to verify access token: %v", err)
	}
	if claims["sub"] != "test-user" {
		t.Errorf("expected subject claim 'test-user', got '%v'", claims["sub"])
	}
}
