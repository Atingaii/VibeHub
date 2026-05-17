// Package store 提供模块共享的数据访问层（DAO）。
//
// 唯一性的真实性来源是 DB 唯一索引；DAO 不做先查后写以避免 TOCTOU 竞态。
package store

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/vibeshop/vibeshop/internal/model"
	"gorm.io/gorm"
)

// ErrIdentifierTaken 表示 username/phone/email 任一已被占用。
// 不区分是哪一列冲突，降低枚举信息泄露面（详见 docs/features/1.1-user-register.md）。
var ErrIdentifierTaken = errors.New("store: identifier taken")

// ErrIdentifierMissing 表示 username/phone/email 三列全为 NULL，违反
// `chk_users_identifier_present` CHECK 约束。这是 service 层校验之外的 DB 兜底，
// 用来保护未来跳过 service 的写入路径（admin / 批量导入 / 其他模块直插）。
var ErrIdentifierMissing = errors.New("store: identifier missing")

// MySQL 错误码：
//   1062 = ER_DUP_ENTRY（唯一键冲突）
//   3819 = ER_CHECK_CONSTRAINT_VIOLATED（CHECK 约束违反，MySQL 8.0+）
// 见 https://dev.mysql.com/doc/mysql-errors/8.0/en/server-error-reference.html
const (
	mysqlDupKey                  = 1062
	mysqlCheckConstraintViolated = 3819
)

// UserStore 用户 DAO。
type UserStore struct {
	db *gorm.DB
}

// NewUserStore 构造 UserStore，绑定到 MySQL 连接。
func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

// Create 写入一条 user 记录。
//
// 入参 u.Username/Phone/Email 中至少一个非 nil，由调用方（service 层）保证；
// DB 层 `chk_users_identifier_present` CHECK 约束作为兜底，违反时返回 ErrIdentifierMissing。
//
// 唯一冲突（MySQL 1062）转为 ErrIdentifierTaken；
// CHECK 违反（MySQL 3819）转为 ErrIdentifierMissing；
// 其他错误原样返回。写入成功后 u.ID / u.CreatedAt / u.UpdatedAt 由 DB 自动填回。
func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			switch mysqlErr.Number {
			case mysqlDupKey:
				return ErrIdentifierTaken
			case mysqlCheckConstraintViolated:
				return ErrIdentifierMissing
			}
		}
		return err
	}
	return nil
}
