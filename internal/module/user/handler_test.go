package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// fakeService 让 handler 单测脱离真 DB / bcrypt。
type fakeService struct {
	// Register 桩
	resp    *RegisterResponse
	err     error
	gotReq  RegisterRequest
	callCnt int

	// Login / Refresh 桩
	loginResp    *LoginResponse
	loginErr     error
	loginGotReq  LoginRequest
	loginCallCnt int

	refreshResp    *LoginResponse
	refreshErr     error
	refreshGotReq  RefreshRequest
	refreshCallCnt int

	// Logout 桩
	logoutErr     error
	logoutGotReq  RefreshRequest
	logoutCallCnt int
}

func (f *fakeService) Register(_ context.Context, req RegisterRequest) (*RegisterResponse, error) {
	f.callCnt++
	f.gotReq = req
	return f.resp, f.err
}

func (f *fakeService) Login(_ context.Context, req LoginRequest) (*LoginResponse, error) {
	f.loginCallCnt++
	f.loginGotReq = req
	return f.loginResp, f.loginErr
}

func (f *fakeService) Refresh(_ context.Context, req RefreshRequest) (*LoginResponse, error) {
	f.refreshCallCnt++
	f.refreshGotReq = req
	return f.refreshResp, f.refreshErr
}

func (f *fakeService) Logout(_ context.Context, req RefreshRequest) error {
	f.logoutCallCnt++
	f.logoutGotReq = req
	return f.logoutErr
}

func newRouterForTest(svc loginService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newHandler(svc)
	r.POST("/api/v1/auth/register", h.Register)
	r.POST("/api/v1/auth/login", h.Login)
	r.POST("/api/v1/auth/refresh", h.Refresh)
	r.POST("/api/v1/auth/logout", h.Logout)
	return r
}

func doPost(t *testing.T, r *gin.Engine, body any) (*httptest.ResponseRecorder, errorBody) {
	t.Helper()
	w := httptest.NewRecorder()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		var err error
		raw, err = json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var eb errorBody
	if w.Code != http.StatusCreated {
		_ = json.Unmarshal(w.Body.Bytes(), &eb)
	}
	return w, eb
}

func TestHandler_Register_201_ReturnsServiceResponse(t *testing.T) {
	uname := "alice"
	svc := &fakeService{
		resp: &RegisterResponse{UserID: 42, Username: &uname},
	}
	r := newRouterForTest(svc)

	w, _ := doPost(t, r, RegisterRequest{Username: "alice", Password: "Strong#1"})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got RegisterResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.UserID != 42 || got.Username == nil || *got.Username != "alice" {
		t.Fatalf("unexpected body: %+v", got)
	}
	if svc.callCnt != 1 {
		t.Fatalf("expected service called once, got %d", svc.callCnt)
	}
}

func TestHandler_Register_400_InvalidIdentifier(t *testing.T) {
	svc := &fakeService{err: ErrInvalidIdentifier}
	r := newRouterForTest(svc)
	w, eb := doPost(t, r, RegisterRequest{Password: "Strong#1"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if eb.Code != "INVALID_REQUEST" {
		t.Fatalf("expected code INVALID_REQUEST, got %q", eb.Code)
	}
}

func TestHandler_Register_400_InvalidPassword(t *testing.T) {
	svc := &fakeService{err: ErrInvalidPassword}
	r := newRouterForTest(svc)
	w, eb := doPost(t, r, RegisterRequest{Username: "alice", Password: "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if eb.Code != "INVALID_REQUEST" {
		t.Fatalf("expected code INVALID_REQUEST, got %q", eb.Code)
	}
}

func TestHandler_Register_409_IdentifierTaken(t *testing.T) {
	svc := &fakeService{err: ErrIdentifierTaken}
	r := newRouterForTest(svc)
	w, eb := doPost(t, r, RegisterRequest{Username: "alice", Password: "Strong#1"})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	if eb.Code != "IDENTIFIER_TAKEN" {
		t.Fatalf("expected code IDENTIFIER_TAKEN, got %q", eb.Code)
	}
}

func TestHandler_Register_500_InternalError(t *testing.T) {
	svc := &fakeService{err: errors.New("db down")}
	r := newRouterForTest(svc)
	w, eb := doPost(t, r, RegisterRequest{Username: "alice", Password: "Strong#1"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if eb.Code != "INTERNAL" {
		t.Fatalf("expected code INTERNAL, got %q", eb.Code)
	}
}

func TestHandler_Register_400_MalformedJSON(t *testing.T) {
	svc := &fakeService{}
	r := newRouterForTest(svc)
	w, eb := doPost(t, r, "{not json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if eb.Code != "INVALID_REQUEST" {
		t.Fatalf("expected code INVALID_REQUEST, got %q", eb.Code)
	}
	if svc.callCnt != 0 {
		t.Fatalf("service must not be called for malformed JSON, got %d", svc.callCnt)
	}
}

// === 1.2 Login / Refresh / Logout HTTP 层 ===

func TestHandler_Login_200(t *testing.T) {
	svc := &fakeService{loginResp: &LoginResponse{
		UserID:       1,
		AccessToken:  "a.b.c",
		RefreshToken: "r.s.t",
		TokenType:    "Bearer",
	}}
	r := newRouterForTest(svc)
	w := httptest.NewRecorder()
	body, _ := json.Marshal(LoginRequest{Identifier: "alice", Password: "Strong#1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var got LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.UserID != 1 || got.AccessToken == "" {
		t.Fatalf("unexpected resp: %+v", got)
	}
}

func TestHandler_Login_400_InvalidIdentifier(t *testing.T) {
	svc := &fakeService{loginErr: ErrInvalidIdentifier}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(LoginRequest{Identifier: "", Password: "Strong#1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandler_Login_401_InvalidCredentials(t *testing.T) {
	svc := &fakeService{loginErr: ErrInvalidCredentials}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(LoginRequest{Identifier: "alice", Password: "wrong#1!"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	var eb errorBody
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if eb.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("got code %q", eb.Code)
	}
}

func TestHandler_Login_500_Internal(t *testing.T) {
	svc := &fakeService{loginErr: errors.New("db down")}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(LoginRequest{Identifier: "alice", Password: "Strong#1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandler_Refresh_200(t *testing.T) {
	svc := &fakeService{refreshResp: &LoginResponse{UserID: 1, AccessToken: "a"}}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(RefreshRequest{RefreshToken: "tok"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.refreshGotReq.RefreshToken != "tok" {
		t.Fatalf("service didn't see token")
	}
}

func TestHandler_Refresh_401_InvalidToken(t *testing.T) {
	svc := &fakeService{refreshErr: ErrInvalidToken}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(RefreshRequest{RefreshToken: "bad"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	var eb errorBody
	_ = json.Unmarshal(w.Body.Bytes(), &eb)
	if eb.Code != "INVALID_TOKEN" {
		t.Fatalf("got %q", eb.Code)
	}
}

func TestHandler_Logout_204(t *testing.T) {
	svc := &fakeService{} // err=nil
	w := httptest.NewRecorder()
	body, _ := json.Marshal(RefreshRequest{RefreshToken: "tok"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body=%s)", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", w.Body.String())
	}
}

func TestHandler_Logout_401_InvalidToken(t *testing.T) {
	svc := &fakeService{logoutErr: ErrInvalidToken}
	w := httptest.NewRecorder()
	body, _ := json.Marshal(RefreshRequest{RefreshToken: "bad"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newRouterForTest(svc).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
