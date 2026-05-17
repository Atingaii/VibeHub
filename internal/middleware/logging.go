package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// sensitiveQueryKeys 列举需要脱敏的 query 参数名（不区分大小写）。
var sensitiveQueryKeys = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"password":      {},
	"passwd":        {},
	"secret":        {},
	"api_key":       {},
	"apikey":        {},
	"key":           {},
	"auth":          {},
	"authorization": {},
	"code":          {},
	"sign":          {},
	"signature":     {},
}

// maskQuery 对 URL query 字符串中的敏感参数值替换为 [REDACTED]。
func maskQuery(raw string) string {
	if raw == "" {
		return ""
	}
	vals, err := url.ParseQuery(raw)
	if err != nil {
		// 解析失败则整体脱敏，不暴露原始内容
		return "[REDACTED]"
	}
	for k := range vals {
		if _, sensitive := sensitiveQueryKeys[strings.ToLower(k)]; sensitive {
			vals[k] = []string{"[REDACTED]"}
		}
	}
	return vals.Encode()
}

// RequestLogger 返回 Gin 请求日志中间件。
// 每个请求结束后记录：method / path / status / latency_ms / client_ip / user_agent。
// query 参数中的敏感字段（token/password/key/secret 等）自动脱敏。
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 请求完成后记录日志
		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.Float64("latency_ms", float64(latency.Nanoseconds())/1e6),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("body_size", c.Writer.Size()),
		}

		if query != "" {
			fields = append(fields, zap.String("query", maskQuery(query)))
		}

		// 如果有错误信息（Gin 通过 c.Error() 记录的）
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		// 按状态码选择日志级别
		switch {
		case status >= 500:
			zap.L().Error("[http] server error", fields...)
		case status >= 400:
			zap.L().Warn("[http] client error", fields...)
		default:
			zap.L().Info("[http] request", fields...)
		}
	}
}

// Recovery 返回 Gin panic 恢复中间件，使用 zap 记录 panic 信息。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		zap.L().Error("[http] panic recovered",
			zap.Any("error", err),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("client_ip", c.ClientIP()),
		)
		c.AbortWithStatus(500)
	})
}
