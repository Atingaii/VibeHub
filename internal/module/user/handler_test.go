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
	resp    *RegisterResponse
	err     error
	gotReq  RegisterRequest
	callCnt int
}

func (f *fakeService) Register(_ context.Context, req RegisterRequest) (*RegisterResponse, error) {
	f.callCnt++
	f.gotReq = req
	return f.resp, f.err
}

func newRouterForTest(svc registerService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newHandler(svc)
	r.POST("/api/v1/auth/register", h.Register)
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
