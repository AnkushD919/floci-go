package cognito

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/floci-io/floci-go/internal/awserr"
)

const (
	cognitoTargetPrefix = "AWSCognitoIdentityProviderService"
	jwtSecret           = "floci-local-jwt-secret-key-12345"
)

type CognitoUser struct {
	Username   string            `json:"Username"`
	Password   string            `json:"-"`
	Attributes map[string]string `json:"Attributes"`
}

type UserPool struct {
	ID    string                  `json:"Id"`
	Name  string                  `json:"Name"`
	Users map[string]*CognitoUser `json:"-"`
}

type CognitoHandler struct {
	mu        sync.RWMutex
	pools     map[string]*UserPool
	AccountID string
}

func NewHandler() *CognitoHandler {
	return &CognitoHandler{
		pools:     make(map[string]*UserPool),
		AccountID: "000000000000",
	}
}

func (h *CognitoHandler) Name() string {
	return "cognito"
}

func (h *CognitoHandler) Matches(r *http.Request) bool {
	target := r.Header.Get("X-Amz-Target")
	if target != "" {
		parts := strings.Split(target, ".")
		return len(parts) > 0 && strings.Contains(strings.ToLower(parts[0]), "cognito")
	}
	return false
}

func (h *CognitoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := r.Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	if len(parts) < 2 {
		awserr.New(400, "MissingTarget", "The target parameter is missing or invalid.").
			WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	action := parts[1]

	switch action {
	case "CreateUserPool":
		h.handleCreateUserPool(w, r)
	case "AdminCreateUser":
		h.handleAdminCreateUser(w, r)
	case "AdminSetUserPassword":
		h.handleAdminSetUserPassword(w, r)
	case "InitiateAuth":
		h.handleInitiateAuth(w, r)
	case "GetUser":
		h.handleGetUser(w, r)
	case "DescribeUserPool":
		h.handleDescribeUserPool(w, r)
	case "DeleteUserPool":
		h.handleDeleteUserPool(w, r)
	default:
		awserr.New(400, "InvalidAction", fmt.Sprintf("Action %s not supported", action)).
			WriteJSONResponse(w, cognitoTargetPrefix)
	}
}

func (h *CognitoHandler) handleCreateUserPool(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PoolName string `json:"PoolName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	poolId := fmt.Sprintf("us-east-1_%s", randomHex(4))
	pool := &UserPool{
		ID:    poolId,
		Name:  input.PoolName,
		Users: make(map[string]*CognitoUser),
	}
	h.pools[poolId] = pool

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"UserPool": pool})
}

func (h *CognitoHandler) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserPoolId     string              `json:"UserPoolId"`
		Username       string              `json:"Username"`
		UserAttributes []map[string]string `json:"UserAttributes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	pool, exists := h.pools[input.UserPoolId]
	if !exists {
		h.mu.Unlock()
		awserr.New(404, "ResourceNotFoundException", "UserPool not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	if _, exists := pool.Users[input.Username]; exists {
		h.mu.Unlock()
		awserr.New(400, "UsernameExistsException", "User already exists").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	attrs := make(map[string]string)
	for _, attr := range input.UserAttributes {
		attrs[attr["Name"]] = attr["Value"]
	}

	user := &CognitoUser{
		Username:   input.Username,
		Attributes: attrs,
	}
	pool.Users[input.Username] = user
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	// Convert attributes back to list of map[string]string for response
	resAttrs := make([]map[string]string, 0, len(attrs))
	for k, v := range attrs {
		resAttrs = append(resAttrs, map[string]string{"Name": k, "Value": v})
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"User": map[string]interface{}{
			"Username":   user.Username,
			"Attributes": resAttrs,
			"Enabled":    true,
		},
	})
}

func (h *CognitoHandler) handleAdminSetUserPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserPoolId string `json:"UserPoolId"`
		Username   string `json:"Username"`
		Password   string `json:"Password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	pool, exists := h.pools[input.UserPoolId]
	if !exists {
		awserr.New(404, "ResourceNotFoundException", "UserPool not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	user, exists := pool.Users[input.Username]
	if !exists {
		awserr.New(404, "UserNotFoundException", "User not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	user.Password = input.Password
	w.WriteHeader(http.StatusOK)
}

func (h *CognitoHandler) handleInitiateAuth(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AuthFlow       string            `json:"AuthFlow"`
		ClientId       string            `json:"ClientId"`
		AuthParameters map[string]string `json:"AuthParameters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	username := input.AuthParameters["USERNAME"]
	password := input.AuthParameters["PASSWORD"]

	h.mu.RLock()
	var foundUser *CognitoUser
	var poolId string
	for pid, pool := range h.pools {
		if u, exists := pool.Users[username]; exists {
			foundUser = u
			poolId = pid
			break
		}
	}
	h.mu.RUnlock()

	if foundUser == nil || subtle.ConstantTimeCompare([]byte(foundUser.Password), []byte(password)) != 1 {
		awserr.New(400, "NotAuthorizedException", "Incorrect username or password").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	// Generate standard HS256 JWT tokens
	now := time.Now()
	exp := now.Add(1 * time.Hour).Unix()

	idClaims := map[string]interface{}{
		"sub":           foundUser.Username,
		"email":         foundUser.Attributes["email"],
		"email_verified": true,
		"iss":           fmt.Sprintf("https://cognito-idp.us-east-1.amazonaws.com/%s", poolId),
		"aud":           input.ClientId,
		"iat":           now.Unix(),
		"exp":           exp,
		"auth_time":     now.Unix(),
	}

	accessClaims := map[string]interface{}{
		"sub":       foundUser.Username,
		"client_id": input.ClientId,
		"iss":       fmt.Sprintf("https://cognito-idp.us-east-1.amazonaws.com/%s", poolId),
		"iat":       now.Unix(),
		"exp":       exp,
	}

	idToken, _ := generateJWT(idClaims, jwtSecret)
	accessToken, _ := generateJWT(accessClaims, jwtSecret)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"AuthenticationResult": map[string]interface{}{
			"AccessToken":  accessToken,
			"ExpiresIn":    3600,
			"IdToken":      idToken,
			"RefreshToken": "mock-refresh-token-" + randomHex(8),
			"TokenType":    "Bearer",
		},
	})
}

func (h *CognitoHandler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Decode claims from mock AccessToken
	claims, err := verifyJWT(input.AccessToken, jwtSecret)
	if err != nil {
		awserr.New(400, "NotAuthorizedException", "Invalid Access Token").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	username, _ := claims["sub"].(string)

	h.mu.RLock()
	var foundUser *CognitoUser
	for _, pool := range h.pools {
		if u, exists := pool.Users[username]; exists {
			foundUser = u
			break
		}
	}
	h.mu.RUnlock()

	if foundUser == nil {
		awserr.New(404, "UserNotFoundException", "User not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	resAttrs := make([]map[string]string, 0, len(foundUser.Attributes))
	for k, v := range foundUser.Attributes {
		resAttrs = append(resAttrs, map[string]string{"Name": k, "Value": v})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"Username":       foundUser.Username,
		"UserAttributes": resAttrs,
	})
}

func (h *CognitoHandler) handleDescribeUserPool(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserPoolId string `json:"UserPoolId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	pool, exists := h.pools[input.UserPoolId]
	h.mu.RUnlock()

	if !exists {
		awserr.New(404, "ResourceNotFoundException", "UserPool not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"UserPool": pool})
}

func (h *CognitoHandler) handleDeleteUserPool(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserPoolId string `json:"UserPoolId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.pools[input.UserPoolId]; !exists {
		awserr.New(404, "ResourceNotFoundException", "UserPool not found").WriteJSONResponse(w, cognitoTargetPrefix)
		return
	}

	delete(h.pools, input.UserPoolId)
	w.WriteHeader(http.StatusOK)
}

func generateJWT(payload map[string]interface{}, secret string) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	payloadBytes, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signingInput := headerB64 + "." + payloadB64

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := mac.Sum(nil)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + sigB64, nil
}

func verifyJWT(token, secret string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token parts")
	}

	signingInput := parts[0] + "." + parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSig := mac.Sum(nil)
	expectedSigB64 := base64.RawURLEncoding.EncodeToString(expectedSig)

	if parts[2] != expectedSigB64 {
		return nil, fmt.Errorf("signature verification failed")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return claims, nil
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

type UserPoolSnapshot struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Users []*CognitoUser `json:"users"`
}

func (h *CognitoHandler) GetPools() []UserPoolSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make([]UserPoolSnapshot, 0, len(h.pools))
	for _, p := range h.pools {
		users := make([]*CognitoUser, 0, len(p.Users))
		for _, u := range p.Users {
			clonedUser := &CognitoUser{
				Username:   u.Username,
				Attributes: make(map[string]string),
			}
			for k, v := range u.Attributes {
				clonedUser.Attributes[k] = v
			}
			users = append(users, clonedUser)
		}
		res = append(res, UserPoolSnapshot{
			ID:    p.ID,
			Name:  p.Name,
			Users: users,
		})
	}
	return res
}
