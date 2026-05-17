package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vibeshop/vibeshop/internal/config"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Manager 管理 MySQL + PostgreSQL 双连接池。
// 遵循 ADR-002: MySQL 存交易型数据，PostgreSQL 存内容型数据。
type Manager struct {
	MySQL    *gorm.DB
	Postgres *gorm.DB
}

// newGormConfig 构造一份独立的 *gorm.Config。每次 Open 必须用独立实例，
// 因为 gorm.Open 会把 *gorm.Config 与 dialector 绑定到返回的 *gorm.DB 上，
// 共享会导致后一次 Open 的方言/clauses 状态污染前一次。
//
// gormLogger 强制 ParameterizedQueries=true：让 SQL 日志渲染占位符而非绑定值，
// 避免 Warn 级别的 SQL 错误日志泄露 password_hash 等敏感参数。
func newGormConfig() *gorm.Config {
	logLevel := gormlogger.Warn
	gormLogger := gormlogger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  true,
		},
	)
	return &gorm.Config{Logger: gormLogger}
}

// OpenMySQL 仅打开 MySQL 连接（启动重试 3 次 + 连接池配置）。
// 用于 migrate 等只需单库的场景，避免连带初始化 PostgreSQL。
func OpenMySQL(cfg config.DBConnConfig) (*gorm.DB, error) {
	zap.L().Info("[database] connecting to MySQL...",
		zap.String("dsn", maskDSN(cfg.DSN)),
	)
	db, err := connectMySQL(cfg, newGormConfig())
	if err != nil {
		return nil, fmt.Errorf("mysql connect: %w", err)
	}
	if err := configurePool(db, cfg); err != nil {
		return nil, fmt.Errorf("mysql pool config: %w", err)
	}
	zap.L().Info("[database] MySQL connected",
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
	)
	return db, nil
}

// OpenPostgres 仅打开 PostgreSQL 连接（启动重试 3 次 + 连接池配置）。
func OpenPostgres(cfg config.DBConnConfig) (*gorm.DB, error) {
	zap.L().Info("[database] connecting to PostgreSQL...",
		zap.String("dsn", maskDSN(cfg.DSN)),
	)
	db, err := connectPostgres(cfg, newGormConfig())
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}
	if err := configurePool(db, cfg); err != nil {
		return nil, fmt.Errorf("postgres pool config: %w", err)
	}
	zap.L().Info("[database] PostgreSQL connected",
		zap.Int("max_open_conns", cfg.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.ConnMaxLifetime),
	)
	return db, nil
}

// CloseGormDB 关闭单个 GORM 连接的底层 *sql.DB。
func CloseGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// New 初始化双数据库连接池。
// 启动时 Ping 验证，失败重试 3 次（间隔 2s），全部失败则返回 error。
func New(cfg *config.DatabaseConfig, debug bool) (*Manager, error) {
	mysqlDB, err := OpenMySQL(cfg.MySQL)
	if err != nil {
		return nil, err
	}
	pgDB, err := OpenPostgres(cfg.Postgres)
	if err != nil {
		_ = CloseGormDB(mysqlDB)
		return nil, err
	}
	return &Manager{
		MySQL:    mysqlDB,
		Postgres: pgDB,
	}, nil
}

// Close 关闭底层数据库连接。
func (m *Manager) Close() error {
	var errs []error

	if sqlDB, err := m.MySQL.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("mysql close: %w", err))
		}
	}

	if sqlDB, err := m.Postgres.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("postgres close: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// PingMySQL 检查 MySQL 连接是否存活。
func (m *Manager) PingMySQL() error {
	sqlDB, err := m.MySQL.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// PingPostgres 检查 PostgreSQL 连接是否存活。
func (m *Manager) PingPostgres() error {
	sqlDB, err := m.Postgres.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// connectMySQL 连接 MySQL，启动时重试 3 次。
func connectMySQL(cfg config.DBConnConfig, gormCfg *gorm.Config) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	for i := range 3 {
		db, err = gorm.Open(mysql.Open(cfg.DSN), gormCfg)
		if err == nil {
			// Ping 验证
			if sqlDB, e := db.DB(); e == nil {
				if e = sqlDB.Ping(); e == nil {
					return db, nil
				}
				err = e
			} else {
				err = e
			}
		}

		zap.L().Warn("[database] MySQL connect failed, retrying...",
			zap.Int("attempt", i+1),
			zap.Error(err),
		)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("after 3 attempts: %w", err)
}

// connectPostgres 连接 PostgreSQL，启动时重试 3 次。
func connectPostgres(cfg config.DBConnConfig, gormCfg *gorm.Config) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	for i := range 3 {
		db, err = gorm.Open(postgres.Open(cfg.DSN), gormCfg)
		if err == nil {
			// Ping 验证
			if sqlDB, e := db.DB(); e == nil {
				if e = sqlDB.Ping(); e == nil {
					return db, nil
				}
				err = e
			} else {
				err = e
			}
		}

		zap.L().Warn("[database] PostgreSQL connect failed, retrying...",
			zap.Int("attempt", i+1),
			zap.Error(err),
		)
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("after 3 attempts: %w", err)
}

// configurePool 配置 database/sql 连接池参数。
func configurePool(db *gorm.DB, cfg config.DBConnConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	configurePoolFromSQLDB(sqlDB, cfg)
	return nil
}

// configurePoolFromSQLDB 在已有 *sql.DB 上配置连接池。
func configurePoolFromSQLDB(sqlDB *sql.DB, cfg config.DBConnConfig) {
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
}

// maskDSN 对 DSN 做脱敏处理，隐藏密码部分。
// MySQL DSN 格式: user:password@tcp(host:port)/db
// PostgreSQL DSN 格式: postgres://user:password@host:port/db
func maskDSN(dsn string) string {
	// 简单处理：找到密码段替换为 ***
	// MySQL: 第一个 ':' 到 '@' 之间
	// PostgreSQL: '://' 后第一个 ':' 到 '@' 之间
	masked := make([]byte, 0, len(dsn))
	inPassword := false
	colonCount := 0
	hasScheme := false

	for i := 0; i < len(dsn); i++ {
		if i+3 <= len(dsn) && dsn[i:i+3] == "://" {
			hasScheme = true
			masked = append(masked, dsn[i:i+3]...)
			i += 2
			colonCount = 0
			continue
		}

		if dsn[i] == ':' && !inPassword {
			colonCount++
			// MySQL: 第一个 ':'（user:pass）；PG: scheme 后第一个 ':'
			if (hasScheme && colonCount == 1) || (!hasScheme && colonCount == 1) {
				inPassword = true
				masked = append(masked, ':')
				masked = append(masked, '*', '*', '*')
				continue
			}
		}

		if dsn[i] == '@' {
			inPassword = false
		}

		if !inPassword {
			masked = append(masked, dsn[i])
		}
	}

	return string(masked)
}
