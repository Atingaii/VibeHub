package user

import "errors"

// 业务错误。service 层返回这些 sentinel；handler 层映射为 HTTP code。
var (
	// ErrInvalidIdentifier 表示 username / phone / email 三选一规则被违反，
	// 或某个具体字段格式不合规。
	ErrInvalidIdentifier = errors.New("user: invalid identifier")

	// ErrInvalidPassword 表示密码长度不在 8-72 之间。
	ErrInvalidPassword = errors.New("user: invalid password")

	// ErrIdentifierTaken 表示某个标识已被占用（DB 唯一索引冲突）。
	ErrIdentifierTaken = errors.New("user: identifier taken")

	// ErrInvalidCredentials 表示用户不存在 OR 密码错（统一文案，缓解枚举）。
	// service.Login 返回；handler 映射为 401 INVALID_CREDENTIALS。
	ErrInvalidCredentials = errors.New("user: invalid credentials")

	// ErrInvalidToken 表示 refresh token 解析失败 / 过期 / typ 不对 / 在 Redis 中已不存在。
	// service.Refresh 返回；handler 映射为 401 INVALID_TOKEN。
	// 注意：logout 路径上仅签名/过期/typ 错才返回此 err，hash 不匹配走幂等 204。
	ErrInvalidToken = errors.New("user: invalid token")
)
