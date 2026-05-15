package database

import (
	"database/sql"
	"fmt"
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

// New 初始化双数据库连接池。
// 启动时 Ping 验证，失败重试 3 次（间隔 2s），全部失败则返回 error。
func New(cfg *config.DatabaseConfig, debug bool) (*Manager, error) {
	// 根据 debug 模式选择 GORM 日志级别。
	// 注意：Info 级别会将完整 SQL（含绑定参数值）输出到日志，存在敏感数据泄露风险。
	// 开发环境使用 Warn 级别：只记录慢查询和错误，不输出完整参数。
	// 如需排查 SQL 问题，可临时改为 gormlogger.Info，但不提交该改动。
	logLevel := gormlogger.Warn

	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(logLevel),
	}

	// 初始化 MySQL
	zap.L().Info("[database] connecting to MySQL...",
		zap.String("dsn", maskDSN(cfg.MySQL.DSN)),
	)
	mysqlDB, err := connectMySQL(cfg.MySQL, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("mysql connect: %w", err)
	}
	if err := configurePool(mysqlDB, cfg.MySQL); err != nil {
		return nil, fmt.Errorf("mysql pool config: %w", err)
	}
	zap.L().Info("[database] MySQL connected",
		zap.Int("max_open_conns", cfg.MySQL.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.MySQL.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.MySQL.ConnMaxLifetime),
	)

	// 初始化 PostgreSQL
	zap.L().Info("[database] connecting to PostgreSQL...",
		zap.String("dsn", maskDSN(cfg.Postgres.DSN)),
	)
	pgDB, err := connectPostgres(cfg.Postgres, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}
	if err := configurePool(pgDB, cfg.Postgres); err != nil {
		return nil, fmt.Errorf("postgres pool config: %w", err)
	}
	zap.L().Info("[database] PostgreSQL connected",
		zap.Int("max_open_conns", cfg.Postgres.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.Postgres.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.Postgres.ConnMaxLifetime),
	)

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
