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

// mysqlDupKey 是 MySQL "Duplicate entry" 的错误码。
// 见 https://dev.mysql.com/doc/mysql-errors/8.0/en/server-error-reference.html#error_er_dup_entry
const mysqlDupKey = 1062

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
// DAO 不重复校验，直接交给 DB 唯一索引兜底。
//
// 唯一冲突（MySQL 1062）转为 ErrIdentifierTaken 返回；其他错误原样返回。
// 写入成功后 u.ID / u.CreatedAt / u.UpdatedAt 由 DB 自动填回。
func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	if err := s.db.WithContext(ctx).Create(u).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDupKey {
			return ErrIdentifierTaken
		}
		return err
	}
	return nil
}
