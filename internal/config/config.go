package config

import "time"

// Config 是应用的顶层配置结构，与 configs/*.yaml 一一对应
type Config struct {
	App           AppConfig           `mapstructure:"app"`
	Gateway       GatewayConfig       `mapstructure:"gateway"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Redis         RedisConfig         `mapstructure:"redis"`
	Messaging     MessagingConfig     `mapstructure:"messaging"`
	OSS           OSSConfig           `mapstructure:"oss"`
	Auth          AuthConfig          `mapstructure:"auth"`
	AI            AIConfig            `mapstructure:"ai"`
	Observability ObservabilityConfig `mapstructure:"observability"`
}

// AppConfig 应用基础信息
type AppConfig struct {
	Name  string `mapstructure:"name"`
	Env   string `mapstructure:"env"`
	Debug bool   `mapstructure:"debug"`
}

// GatewayConfig HTTP 网关配置
type GatewayConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// DatabaseConfig 数据库配置（双库）
type DatabaseConfig struct {
	MySQL    DBConnConfig `mapstructure:"mysql"`
	Postgres DBConnConfig `mapstructure:"postgres"`
}

// DBConnConfig 单个数据库连接配置
type DBConnConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// RedisConfig Redis 配置。
//
// 单实例阶段：所有 Pool 共享同一个 server，DB 由 Pool 拓扑（internal/cache/keys.go）
// 决定。这里不再暴露 db 字段，避免和 Pool 拓扑形成双重来源（见 ADR-003）。
type RedisConfig struct {
	Addr         string `mapstructure:"addr"`
	Password     string `mapstructure:"password"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// MessagingConfig 消息队列配置
type MessagingConfig struct {
	NATS NATSConfig `mapstructure:"nats"`
}

// NATSConfig NATS 连接配置
type NATSConfig struct {
	URL       string `mapstructure:"url"`
	JetStream bool   `mapstructure:"jetstream"`
}

// OSSConfig 对象存储配置
type OSSConfig struct {
	Provider  string `mapstructure:"provider"`
	LocalPath string `mapstructure:"local_path"`
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	JWTSecret       string        `mapstructure:"jwt_secret"`
	AccessTokenTTL  time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
}

// AIConfig AI 相关配置
type AIConfig struct {
	DefaultModel    string `mapstructure:"default_model"`
	OllamaURL       string `mapstructure:"ollama_url"`
	OpenAIAPIKey    string `mapstructure:"openai_api_key"`
	ClaudeAPIKey    string `mapstructure:"claude_api_key"`
	DailyTokenBudget int   `mapstructure:"daily_token_budget"`
}

// ObservabilityConfig 可观测性配置
type ObservabilityConfig struct {
	LogLevel  string        `mapstructure:"log_level"`
	LogFormat string        `mapstructure:"log_format"`
	Tracing   TracingConfig `mapstructure:"tracing"`
	Metrics   MetricsConfig `mapstructure:"metrics"`
}

// TracingConfig 链路追踪配置
type TracingConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Exporter string `mapstructure:"exporter"`
	Endpoint string `mapstructure:"endpoint"`
}

// MetricsConfig 指标采集配置
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}
