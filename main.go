package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/vibeshop/vibeshop/internal/cache"
	"github.com/vibeshop/vibeshop/internal/config"
	"github.com/vibeshop/vibeshop/internal/database"
	"github.com/vibeshop/vibeshop/internal/logger"
	"github.com/vibeshop/vibeshop/internal/migrate"
	"github.com/vibeshop/vibeshop/internal/server"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Version 和 BuildTime 由 ldflags 注入
var (
	Version   = "dev"
	BuildTime = "unknown"
)

//go:embed all:scripts/migration
var migrationFS embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrate(os.Args[2:]); err != nil {
			_, _ = os.Stderr.WriteString("[fatal] migrate: " + err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	server.Version = Version
	server.BuildTime = BuildTime

	cfg, err := config.Load("configs")
	if err != nil {
		_, _ = os.Stderr.WriteString("[fatal] failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	if err := logger.Init(cfg.Observability.LogLevel, cfg.Observability.LogFormat); err != nil {
		_, _ = os.Stderr.WriteString("[fatal] failed to init logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer logger.Sync()

	zap.L().Info("[main] VibeShop starting",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
		zap.String("env", cfg.App.Env),
		zap.Int("port", cfg.Gateway.Port),
		zap.Bool("debug", cfg.App.Debug),
	)

	dbManager, err := database.New(&cfg.Database, cfg.App.Debug)
	if err != nil {
		zap.L().Fatal("[main] failed to init database", zap.Error(err))
	}
	defer func() {
		if err := dbManager.Close(); err != nil {
			zap.L().Error("[main] database close error", zap.Error(err))
		}
	}()

	redisManager, err := cache.NewRedis(&cfg.Redis)
	if err != nil {
		zap.L().Fatal("[main] failed to init redis", zap.Error(err))
	}
	defer func() {
		if err := redisManager.Close(); err != nil {
			zap.L().Error("[main] redis close error", zap.Error(err))
		}
	}()

	srv := server.NewServer(cfg, dbManager, redisManager)
	if err := srv.Run(); err != nil {
		zap.L().Fatal("[main] server error", zap.Error(err))
	}
}

// runMigrate 处理 `vibeshop migrate [up|down|status] [mysql|pg|all]` 子命令。
//
// 解析规则：
//   - migrate                → up all
//   - migrate up             → up all
//   - migrate up <target>    → up <target>
//   - migrate down [target]  → down <target | all>
//   - migrate status [target]→ status <target | all>
//
// 子命令独立加载 config，但**只**为请求的 target 打开 DB（迁移目录非空时）。
// 这样 `migrate up pg` 不会因为 MySQL 不可达失败，反之亦然。
func runMigrate(args []string) error {
	action := migrate.ActionUp
	targetStr := ""
	switch len(args) {
	case 0:
		// migrate → up all
	case 1:
		switch args[0] {
		case "up":
			action = migrate.ActionUp
		case "down":
			action = migrate.ActionDown
		case "status":
			action = migrate.ActionStatus
		case "mysql", "pg", "all":
			targetStr = args[0]
		default:
			return fmt.Errorf("unknown migrate arg %q (want up|down|status or mysql|pg|all)", args[0])
		}
	case 2:
		switch args[0] {
		case "up":
			action = migrate.ActionUp
		case "down":
			action = migrate.ActionDown
		case "status":
			action = migrate.ActionStatus
		default:
			return fmt.Errorf("unknown migrate action %q (want up|down|status)", args[0])
		}
		targetStr = args[1]
	default:
		return errors.New("usage: migrate [up|down|status] [mysql|pg|all]")
	}

	target, err := migrate.ParseTarget(targetStr)
	if err != nil {
		return err
	}

	cfg, err := config.Load("configs")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := logger.Init(cfg.Observability.LogLevel, cfg.Observability.LogFormat); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync()

	provider := &lazyDBProvider{cfg: cfg}
	defer provider.closeAll()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch action {
	case migrate.ActionUp:
		return migrate.Up(ctx, provider, migrationFS, target)
	case migrate.ActionDown:
		return migrate.Down(ctx, provider, migrationFS, target)
	case migrate.ActionStatus:
		return migrate.Status(ctx, provider, migrationFS, target)
	default:
		return fmt.Errorf("unreachable: action=%q", action)
	}
}

// lazyDBProvider 实现 migrate.DBProvider：第一次被请求时才打开对应的库。
// 这样空目录跳过的场景完全不会触发连接，单库 migrate 也不会强制初始化另一库。
type lazyDBProvider struct {
	cfg     *config.Config
	mysqlDB *gorm.DB
	pgDB    *gorm.DB
}

func (p *lazyDBProvider) DB(target migrate.Target) (*sql.DB, error) {
	switch target {
	case migrate.TargetMySQL:
		if p.mysqlDB == nil {
			db, err := database.OpenMySQL(p.cfg.Database.MySQL)
			if err != nil {
				return nil, err
			}
			p.mysqlDB = db
		}
		return p.mysqlDB.DB()
	case migrate.TargetPG:
		if p.pgDB == nil {
			db, err := database.OpenPostgres(p.cfg.Database.Postgres)
			if err != nil {
				return nil, err
			}
			p.pgDB = db
		}
		return p.pgDB.DB()
	default:
		return nil, fmt.Errorf("lazyDBProvider: unsupported target %q", target)
	}
}

func (p *lazyDBProvider) closeAll() {
	if p.mysqlDB != nil {
		if err := database.CloseGormDB(p.mysqlDB); err != nil {
			zap.L().Error("[main] migrate close mysql", zap.Error(err))
		}
	}
	if p.pgDB != nil {
		if err := database.CloseGormDB(p.pgDB); err != nil {
			zap.L().Error("[main] migrate close postgres", zap.Error(err))
		}
	}
}
