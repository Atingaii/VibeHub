// Package migrate 封装数据库 schema 迁移（goose 库模式）。
//
// 设计要点（详见 docs/features/1.1-user-register.md）：
//   - 同时管理 MySQL 与 PostgreSQL 两个目录，按 Target 选择执行范围。
//   - SQL 文件由调用方通过 embed.FS 注入，避免依赖宿主机源码目录。
//   - 单库失败立即返回，已成功的库不跨库回滚（goose 本身已逐文件提交）。
//   - 空目录视为零迁移版本，Up/Down/Status 均按 no-op 处理并返回 nil。
//   - DBProvider 由调用方按 target 懒加载 *sql.DB，保证单库命令不会强制开两个库。
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

// Target 标识本次迁移作用的数据库目标。
type Target string

const (
	TargetMySQL Target = "mysql"
	TargetPG    Target = "pg"
	TargetAll   Target = "all"
)

// ParseTarget 解析命令行字符串到 Target；空串等价 TargetAll。
func ParseTarget(s string) (Target, error) {
	switch s {
	case "", string(TargetAll):
		return TargetAll, nil
	case string(TargetMySQL):
		return TargetMySQL, nil
	case string(TargetPG):
		return TargetPG, nil
	default:
		return "", fmt.Errorf("unknown migrate target %q (want mysql|pg|all)", s)
	}
}

// Action 表示一次迁移命令的方向。
type Action string

const (
	ActionUp     Action = "up"
	ActionDown   Action = "down"
	ActionStatus Action = "status"
)

// DBProvider 由调用方实现，按 target 按需返回 *sql.DB。
//
// 关键约定：runOne 仅在确认目录非空后才会调用 DBProvider，
// 所以单库 migrate（例如 `migrate up pg`）不会因为另一库不可达而失败。
// 实现方对 *sql.DB 的生命周期负责（migrate 本身不 Close）。
type DBProvider interface {
	DB(target Target) (*sql.DB, error)
}

// Up 应用所有未执行的迁移。
func Up(ctx context.Context, p DBProvider, srcFS embed.FS, target Target) error {
	return runAction(ctx, p, srcFS, target, ActionUp)
}

// Down 回滚一步迁移。
func Down(ctx context.Context, p DBProvider, srcFS embed.FS, target Target) error {
	return runAction(ctx, p, srcFS, target, ActionDown)
}

// Status 打印迁移状态到 stdout（goose 默认行为）。
func Status(ctx context.Context, p DBProvider, srcFS embed.FS, target Target) error {
	return runAction(ctx, p, srcFS, target, ActionStatus)
}

func runAction(ctx context.Context, p DBProvider, srcFS embed.FS, target Target, act Action) error {
	switch target {
	case TargetMySQL, TargetPG:
		return runOne(ctx, p, srcFS, target, act)
	case TargetAll:
		if err := runOne(ctx, p, srcFS, TargetMySQL, act); err != nil {
			return err
		}
		return runOne(ctx, p, srcFS, TargetPG, act)
	default:
		return fmt.Errorf("invalid target %q", target)
	}
}

// gooseGlobalLock 串行化 goose 全局状态（SetBaseFS / SetDialect）的并发访问。
// 当前 main.go 中 migrate 子命令是顺序调用，但加锁防止未来误用导致两个 target 抢全局状态。
var gooseGlobalLock sync.Mutex

// runOne 在单一 target 上执行一次 goose 命令。
func runOne(ctx context.Context, p DBProvider, srcFS embed.FS, target Target, act Action) error {
	dialect, dir, err := resolveDialectDir(target)
	if err != nil {
		return err
	}

	empty, err := dirHasNoMigrations(srcFS, dir)
	if err != nil {
		return fmt.Errorf("scan migration dir %q: %w", dir, err)
	}
	if empty {
		zap.L().Info("[migrate] no migrations found, skipping",
			zap.String("target", string(target)),
			zap.String("action", string(act)),
			zap.String("dir", dir),
		)
		return nil
	}

	sqlDB, err := p.DB(target)
	if err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}

	gooseGlobalLock.Lock()
	defer gooseGlobalLock.Unlock()

	goose.SetBaseFS(srcFS)
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("goose dialect (%s): %w", target, err)
	}

	t0 := time.Now()
	zap.L().Info("[migrate] running",
		zap.String("target", string(target)),
		zap.String("action", string(act)),
		zap.String("dialect", dialect),
		zap.String("dir", dir),
	)

	switch act {
	case ActionUp:
		err = goose.UpContext(ctx, sqlDB, dir)
	case ActionDown:
		err = goose.DownContext(ctx, sqlDB, dir)
	case ActionStatus:
		err = goose.StatusContext(ctx, sqlDB, dir)
	default:
		return fmt.Errorf("unknown action %q", act)
	}
	if err != nil {
		return fmt.Errorf("goose %s on %s: %w", act, target, err)
	}

	zap.L().Info("[migrate] done",
		zap.String("target", string(target)),
		zap.String("action", string(act)),
		zap.Duration("elapsed", time.Since(t0)),
	)
	return nil
}

// resolveDialectDir 把 Target 映射到 (dialect, dir)。
func resolveDialectDir(target Target) (string, string, error) {
	switch target {
	case TargetMySQL:
		return "mysql", "scripts/migration/mysql", nil
	case TargetPG:
		return "postgres", "scripts/migration/pg", nil
	default:
		return "", "", fmt.Errorf("resolve: unsupported target %q", target)
	}
}

// dirHasNoMigrations 判定一个目录里是否没有任何 .sql 迁移文件。
// 空目录 / 仅 .gitkeep / 仅非 .sql 文件 都视为 no-op。
func dirHasNoMigrations(srcFS embed.FS, dir string) (bool, error) {
	entries, err := fs.ReadDir(srcFS, dir)
	if err != nil {
		// 目录不存在也按 no-op 处理（开发期手动删过目录的兜底）。
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			return false, nil
		}
	}
	return true, nil
}
