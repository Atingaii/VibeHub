// Package model 存放跨模块共享的数据库实体。
package model

import "time"

// User 对应 MySQL `users` 表。
//
// username / phone / email 用 *string：
//   - nil 表示该列为 NULL（MySQL 唯一索引允许多个 NULL，恰好支撑"任一标识即可注册"）
//   - 非空字符串表示该列已被该用户占用
//
// 表创建依赖 scripts/migration/mysql/00001_create_users.sql 显式 DDL，
// 不依赖 GORM AutoMigrate；此处的 gorm tag 仅用于查询/插入时的列名对齐。
type User struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Username     *string   `gorm:"column:username"`
	Phone        *string   `gorm:"column:phone"`
	Email        *string   `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	Status       int8      `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

// User 状态常量。1.1 仅写 StatusActive；StatusDisabled 留给后续下线流程。
const (
	UserStatusActive   int8 = 1
	UserStatusDisabled int8 = 2
)

// TableName 显式指定表名，避免 GORM 复数化导致和 DDL 不一致。
func (User) TableName() string { return "users" }
