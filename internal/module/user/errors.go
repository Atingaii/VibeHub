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
)
