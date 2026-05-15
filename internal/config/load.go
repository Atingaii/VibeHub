package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load 从配置文件加载配置，支持环境变量覆盖。
//
// 配置文件选择优先级：
//  1. 环境变量 APP_ENV 指定环境 → configs/{APP_ENV}.yaml
//  2. 默认 → configs/dev.yaml
//
// 环境变量覆盖规则：
//   - Viper AutomaticEnv + EnvKeyReplacer("." → "_")
//   - 例：gateway.port → GATEWAY_PORT
//   - 显式绑定：APP_PORT → gateway.port（兼容 0.1 阶段约定）
func Load(configDir string) (*Config, error) {
	v := viper.New()

	// 确定环境
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}

	// 配置文件路径
	v.SetConfigName(env)
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	// 环境变量覆盖支持
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 显式绑定常用环境变量（兼容 docs/dev-workflow.md 中定义的变量名）
	bindEnvMappings(v)

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file (%s/%s.yaml): %w", configDir, env, err)
	}

	// 解析到 struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

// bindEnvMappings 显式绑定环境变量到配置路径。
// 这些映射与 docs/dev-workflow.md 环境变量参考表对齐。
func bindEnvMappings(v *viper.Viper) {
	mappings := map[string]string{
		// App
		"app.env": "APP_ENV",

		// Gateway — APP_PORT 是 0.1 阶段约定的变量名，保持兼容
		"gateway.port": "APP_PORT",

		// Database
		"database.mysql.dsn":    "MYSQL_DSN",
		"database.postgres.dsn": "PG_DSN",

		// Redis
		"redis.addr":     "REDIS_ADDR",
		"redis.password": "REDIS_PASSWORD",

		// Messaging
		"messaging.nats.url": "NATS_URL",

		// OSS
		"oss.provider":   "OSS_PROVIDER",
		"oss.local_path": "OSS_LOCAL_PATH",
		"oss.endpoint":   "OSS_ENDPOINT",
		"oss.access_key": "OSS_ACCESS_KEY",
		"oss.secret_key": "OSS_SECRET_KEY",
		"oss.bucket":     "OSS_BUCKET",

		// Auth
		"auth.jwt_secret":        "JWT_SECRET",
		"auth.access_token_ttl":  "JWT_ACCESS_TTL",
		"auth.refresh_token_ttl": "JWT_REFRESH_TTL",

		// AI
		"ai.default_model":      "AI_DEFAULT_MODEL",
		"ai.ollama_url":         "OLLAMA_URL",
		"ai.openai_api_key":     "OPENAI_API_KEY",
		"ai.claude_api_key":     "CLAUDE_API_KEY",
		"ai.daily_token_budget": "AI_DAILY_TOKEN_BUDGET",

		// Observability
		"observability.log_level":          "LOG_LEVEL",
		"observability.log_format":         "LOG_FORMAT",
		"observability.tracing.endpoint":   "OTEL_EXPORTER_ENDPOINT",
		"observability.metrics.port":       "METRICS_PORT",
	}

	for key, envVar := range mappings {
		_ = v.BindEnv(key, envVar)
	}
}
