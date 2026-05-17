package user

import "time"

// RegisterRequest 是 POST /api/v1/auth/register 的入参。
//
// username / phone / email 三者**有且仅有一个**非空字符串，否则 400。
// 入参用 string（非指针）：JSON 缺省值即空串，service 层先 TrimSpace 再判空。
type RegisterRequest struct {
	Username string `json:"username"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterResponse 是注册成功的响应体。
//
// username / phone / email 用 *string 直接映射到 DB 列：未占用的列在响应里渲染为 null。
type RegisterResponse struct {
	UserID    uint64    `json:"user_id"`
	Username  *string   `json:"username"`
	Phone     *string   `json:"phone"`
	Email     *string   `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// errorBody 是统一的错误响应体（{"code", "message"}）。
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LoginRequest 是 POST /api/v1/auth/login 的入参。
//
// identifier 单字段，service 层 classifyIdentifier 识别为 username / phone / email 之一。
type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// LoginResponse 是登录 / refresh 成功的响应。
type LoginResponse struct {
	UserID           uint64    `json:"user_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	TokenType        string    `json:"token_type"`
}

// RefreshRequest 是 /refresh 与 /logout 的入参。
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
