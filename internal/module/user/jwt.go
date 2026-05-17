package user

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// MinJWTSecretBytes 是 JWT secret 的最小长度（HS256 安全边界）。
// main.go 启动时会强制校验。
const MinJWTSecretBytes = 32

// TokenType 标识 JWT 用途，写入 claims 的 "typ" 字段以防混用（access ↔ refresh）。
type TokenType string

const (
	TokenAccess  TokenType = "access"
	TokenRefresh TokenType = "refresh"
)

const jwtIssuer = "vibeshop"

// Claims 是 access / refresh token 共用的 claims 类型。
type Claims struct {
	Type TokenType `json:"typ"`
	JTI  string    `json:"jti,omitempty"` // 仅 refresh 用
	jwt.RegisteredClaims
}

// JWTSigner 签发与解析 HS256 JWT。
type JWTSigner struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time // 测试可注入
}

// NewJWTSigner 构造 signer；secret 必须 ≥ MinJWTSecretBytes。
func NewJWTSigner(secret []byte, accessTTL, refreshTTL time.Duration) (*JWTSigner, error) {
	if len(secret) < MinJWTSecretBytes {
		return nil, fmt.Errorf("jwt secret too short: %d bytes (need >= %d)", len(secret), MinJWTSecretBytes)
	}
	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, errors.New("jwt: access/refresh TTL must be positive")
	}
	return &JWTSigner{
		secret:     secret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}, nil
}

// SignAccess 签发 access token。
func (s *JWTSigner) SignAccess(uid uint64) (token string, expiresAt time.Time, err error) {
	expiresAt = s.now().Add(s.accessTTL)
	claims := Claims{
		Type: TokenAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   strconv.FormatUint(uid, 10),
			IssuedAt:  jwt.NewNumericDate(s.now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	return token, expiresAt, err
}

// SignRefresh 签发 refresh token；jti 由调用方提供（与 Redis key 中的 <jti> 一致）。
func (s *JWTSigner) SignRefresh(uid uint64, jti string) (token string, expiresAt time.Time, err error) {
	expiresAt = s.now().Add(s.refreshTTL)
	claims := Claims{
		Type: TokenRefresh,
		JTI:  jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   strconv.FormatUint(uid, 10),
			IssuedAt:  jwt.NewNumericDate(s.now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	return token, expiresAt, err
}

// Parse 解析并验证 token；要求 typ == expected。
// 返回 sub (user id) + jti（refresh 才有）+ exp。
func (s *JWTSigner) Parse(tokenStr string, expected TokenType) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected alg: %s", t.Method.Alg())
		}
		return s.secret, nil
	}, jwt.WithIssuer(jwtIssuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("jwt: invalid claims")
	}
	if claims.Type != expected {
		return nil, fmt.Errorf("jwt: type mismatch (got %q, want %q)", claims.Type, expected)
	}
	return claims, nil
}

// SubjectAsUserID 把 claims.Subject 解析为 uint64。
func (c *Claims) SubjectAsUserID() (uint64, error) {
	return strconv.ParseUint(c.Subject, 10, 64)
}

// NewJTI 生成 16 字节随机 base32-nopad 字符串（26 字符）。
// 失败概率极低（crypto/rand），调用方按内部错误处理。
func NewJTI() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("jwt: rand: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:]), nil
}
