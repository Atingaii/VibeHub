package user

import (
	"strings"
	"testing"
	"time"
)

func newTestSigner(t *testing.T) *JWTSigner {
	t.Helper()
	secret := []byte(strings.Repeat("a", MinJWTSecretBytes))
	s, err := NewJWTSigner(secret, 2*time.Hour, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("NewJWTSigner: %v", err)
	}
	return s
}

func TestNewJWTSigner_RejectsShortSecret(t *testing.T) {
	_, err := NewJWTSigner([]byte("short"), time.Hour, time.Hour)
	if err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestNewJWTSigner_RejectsZeroTTL(t *testing.T) {
	secret := []byte(strings.Repeat("a", MinJWTSecretBytes))
	_, err := NewJWTSigner(secret, 0, time.Hour)
	if err == nil {
		t.Fatal("expected error for zero access TTL")
	}
}

func TestSignAccess_RoundTrip(t *testing.T) {
	s := newTestSigner(t)
	tok, exp, err := s.SignAccess(42)
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	if tok == "" || time.Until(exp) < time.Hour {
		t.Fatalf("unexpected output: tok=%q exp=%v", tok, exp)
	}

	claims, err := s.Parse(tok, TokenAccess)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	uid, err := claims.SubjectAsUserID()
	if err != nil || uid != 42 {
		t.Fatalf("expected uid 42, got %d (err=%v)", uid, err)
	}
	if claims.Type != TokenAccess {
		t.Fatalf("expected typ=access, got %q", claims.Type)
	}
}

func TestSignRefresh_RoundTrip(t *testing.T) {
	s := newTestSigner(t)
	jti, err := NewJTI()
	if err != nil {
		t.Fatalf("NewJTI: %v", err)
	}
	if len(jti) != 26 {
		t.Fatalf("expected 26-char jti, got %d", len(jti))
	}
	tok, _, err := s.SignRefresh(42, jti)
	if err != nil {
		t.Fatalf("SignRefresh: %v", err)
	}
	claims, err := s.Parse(tok, TokenRefresh)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.JTI != jti {
		t.Fatalf("jti mismatch: %q vs %q", claims.JTI, jti)
	}
}

// 防混用：access token 当 refresh 解析必须报错；反之亦然。
func TestParse_TypMismatch(t *testing.T) {
	s := newTestSigner(t)
	access, _, _ := s.SignAccess(1)
	if _, err := s.Parse(access, TokenRefresh); err == nil {
		t.Fatal("expected typ mismatch when parsing access as refresh")
	}
	jti, _ := NewJTI()
	refresh, _, _ := s.SignRefresh(1, jti)
	if _, err := s.Parse(refresh, TokenAccess); err == nil {
		t.Fatal("expected typ mismatch when parsing refresh as access")
	}
}

func TestParse_RejectsExpired(t *testing.T) {
	secret := []byte(strings.Repeat("a", MinJWTSecretBytes))
	s, _ := NewJWTSigner(secret, time.Second, time.Hour)
	// 把"现在"调到过去，签出的 token 立即过期。
	s.now = func() time.Time { return time.Now().Add(-time.Hour) }
	tok, _, _ := s.SignAccess(1)
	// 还原时间继续验证
	s.now = time.Now
	if _, err := s.Parse(tok, TokenAccess); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestParse_RejectsTamperedSignature(t *testing.T) {
	s := newTestSigner(t)
	tok, _, _ := s.SignAccess(1)
	// 修改最后一个字符；用 'A'/'B' 切换确保改动一定生效（base64url 字母表两者都合法）。
	last := tok[len(tok)-1]
	repl := byte('A')
	if last == 'A' {
		repl = 'B'
	}
	tampered := tok[:len(tok)-1] + string(repl)
	if _, err := s.Parse(tampered, TokenAccess); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestParse_RejectsWrongAlgorithm(t *testing.T) {
	// 一个 alg=none 的 token（构造攻击）
	s := newTestSigner(t)
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJpc3MiOiJ2aWJlc2hvcCIsInN1YiI6IjEifQ."
	if _, err := s.Parse(noneToken, TokenAccess); err == nil {
		t.Fatal("expected error for alg=none token")
	}
}
