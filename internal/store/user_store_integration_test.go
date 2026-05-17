package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/vibeshop/vibeshop/internal/config"
	"github.com/vibeshop/vibeshop/internal/database"
	"github.com/vibeshop/vibeshop/internal/model"
	"go.uber.org/zap"
)

// 集成测试，依赖真实 MySQL（chk_users_identifier_present CHECK 约束需要 MySQL 8.0+）。
// 设置 VIBESHOP_INTEGRATION_MYSQL_DSN 环境变量启用；未设置时 t.Skip。
//
// 例：
//
//	VIBESHOP_INTEGRATION_MYSQL_DSN="vibeshop:xxx@tcp(127.0.0.1:3307)/vibeshop?charset=utf8mb4&parseTime=True&loc=Local" \
//	  go test ./internal/store/...
func TestUserStore_Create_RejectsAllNullIdentifiers(t *testing.T) {
	dsn := os.Getenv("VIBESHOP_INTEGRATION_MYSQL_DSN")
	if dsn == "" {
		t.Skip("VIBESHOP_INTEGRATION_MYSQL_DSN not set; skipping integration test")
	}
	// 替换全局 zap，避免 [database] 日志写到测试输出。
	prev := zap.ReplaceGlobals(zap.NewNop())
	defer prev()

	db, err := database.OpenMySQL(config.DBConnConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	defer func() { _ = database.CloseGormDB(db) }()

	store := NewUserStore(db)

	// 直接绕过 service.normalizeAndValidate 写入：模拟 "未来某个写入路径忘了校验" 的场景。
	ghost := &model.User{
		Username:     nil,
		Phone:        nil,
		Email:        nil,
		PasswordHash: "$2a$10$" + string(make([]byte, 53)), // 占位 60 字符
		Status:       model.UserStatusActive,
	}
	err = store.Create(context.Background(), ghost)
	if !errors.Is(err, ErrIdentifierMissing) {
		t.Fatalf("expected ErrIdentifierMissing for all-NULL identifiers, got %v", err)
	}
	if ghost.ID != 0 {
		t.Fatalf("ghost row should not have been written, but got ID=%d", ghost.ID)
	}
}
