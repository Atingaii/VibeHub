package user

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/vibeshop/vibeshop/internal/model"
	"github.com/vibeshop/vibeshop/internal/store"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// userStore 是 service 对 DAO 的最小依赖契约（便于单测 mock）。
type userStore interface {
	Create(ctx context.Context, u *model.User) error
	FindByIdentifier(ctx context.Context, norm string, idType store.IdentifierType) (*model.User, error)
	FindByID(ctx context.Context, id uint64) (*model.User, error)
}

// service 封装注册 / 登录 / refresh / logout 的业务逻辑。
type service struct {
	store   userStore
	signer  *JWTSigner
	refresh RefreshStore
}

// newService 构造 service。signer / refresh 在 1.2 引入；1.1 单独构造时可传 nil
// （注册流程不依赖这两者；测试也方便）。
func newService(s userStore, signer *JWTSigner, refresh RefreshStore) *service {
	return &service{store: s, signer: signer, refresh: refresh}
}

const (
	bcryptCost = 10
	pwdMinLen  = 8
	pwdMaxLen  = 72 // bcrypt 上限
)

var (
	// usernameRE 要求 4-32 字符 + 至少一个非数字字符。
	// 加非数字约束的原因：identifier 单字段登录时按"含 @ → email；纯数字 → phone；
	// 其他 → username"识别，纯数字 username 会和 phone 撞值（13800138000 永远走 phone 分支），
	// 导致"能注册却登不上"的歧义。1.2 阶段决定收紧产品规则禁止纯数字 username。
	usernameRE  = regexp.MustCompile(`^(?:[a-z0-9_-]*[a-z_-][a-z0-9_-]*)$`)
	usernameLen = func(s string) bool { return len(s) >= 4 && len(s) <= 32 }
	phoneRE     = regexp.MustCompile(`^\d{6,20}$`)
	// emailRE 要求 local-part 至少一个字符 + '@' + domain（含至少一个 '.'，TLD 至少 2 字符）。
	// 配合 net/mail.ParseAddress 严格校验，拒绝 display-name 形式（"Foo <a@b>"）和无 TLD 形式（"a@b"）。
	emailRE = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

// validUsername 复合校验：先 4-32 长度，再检查正则（含至少一个非数字）。
func validUsername(s string) bool {
	return usernameLen(s) && usernameRE.MatchString(s)
}

// normalizedReq 是 RegisterRequest 经规范化后的形态。
// 三个标识字段均为指针：nil 表示该字段未提供。
type normalizedReq struct {
	username *string
	phone    *string
	email    *string
	password string
	idType   string // "username" | "phone" | "email"，仅供日志
}

// Register 创建一个新用户。
//
// 流程：normalize → validate → bcrypt → store.Create。
// 错误统一为模块 sentinel（ErrInvalidIdentifier / ErrInvalidPassword / ErrIdentifierTaken），
// 由 handler 转 HTTP code。
func (s *service) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	n, reason, err := normalizeAndValidate(req)
	if err != nil {
		zap.L().Info("[user] register fail",
			zap.String("code", "INVALID_REQUEST"),
			zap.String("reason", reason),
		)
		return nil, err
	}

	zap.L().Info("[user] register attempt",
		zap.String("identifier_type", n.idType),
	)

	hash, err := bcrypt.GenerateFromPassword([]byte(n.password), bcryptCost)
	if err != nil {
		// bcrypt 自身失败属于内部错误，原样返回（handler 转 500）。
		return nil, err
	}

	u := &model.User{
		Username:     n.username,
		Phone:        n.phone,
		Email:        n.email,
		PasswordHash: string(hash),
		Status:       model.UserStatusActive,
	}

	if err := s.store.Create(ctx, u); err != nil {
		if errors.Is(err, store.ErrIdentifierTaken) {
			zap.L().Info("[user] register fail",
				zap.String("code", "IDENTIFIER_TAKEN"),
				zap.String("identifier_type", n.idType),
			)
			return nil, ErrIdentifierTaken
		}
		// DB 层 CHECK 兜底：service 校验已拦三选一，理论上走不到这里。
		// 真触发说明上游绕过了 service.normalizeAndValidate（admin 工具 / 批量导入 / 测试），
		// 仍按 INVALID_REQUEST 对外，但内部记 "db_check_violated" 便于排查。
		if errors.Is(err, store.ErrIdentifierMissing) {
			zap.L().Warn("[user] register fail",
				zap.String("code", "INVALID_REQUEST"),
				zap.String("reason", "db_check_violated"),
				zap.String("identifier_type", n.idType),
			)
			return nil, ErrInvalidIdentifier
		}
		return nil, err
	}

	zap.L().Info("[user] register success",
		zap.Uint64("user_id", u.ID),
		zap.String("identifier_type", n.idType),
	)

	return &RegisterResponse{
		UserID:    u.ID,
		Username:  u.Username,
		Phone:     u.Phone,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}, nil
}

// identifierKind 区分 username / phone / email；与 store.IdentifierType 一一对应。
type identifierKind string

const (
	kindUsername identifierKind = "username"
	kindPhone    identifierKind = "phone"
	kindEmail    identifierKind = "email"
)

// classifyIdentifier 把单一 identifier 字符串识别 + 规范化。
//
// 1.1 注册（三独立字段）和 1.2 登录（单 identifier 字段）共用此 helper，
// 保证两侧的 normalize / format 规则严格一致——避免一边能注册另一边却查不到。
//
// 输入：原始字符串（未 TrimSpace）。
// 输出：(规范化后的字符串, kind, err)；err 仅当格式不合规时非 nil。
//
// 识别策略：先按内容判 kind（含 '@' → email；纯数字 → phone；其他 → username），
// 再按 kind 走对应正则 / 邮箱严格校验。这样调用方无需事先知道 identifier 是哪种。
func classifyIdentifier(s string) (string, identifierKind, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", ErrInvalidIdentifier
	}
	switch {
	case strings.Contains(s, "@"):
		email := strings.ToLower(s)
		if !emailRE.MatchString(email) {
			return "", "", ErrInvalidIdentifier
		}
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Name != "" || addr.Address != email {
			return "", "", ErrInvalidIdentifier
		}
		return email, kindEmail, nil
	case phoneRE.MatchString(s):
		return s, kindPhone, nil
	default:
		uname := strings.ToLower(s)
		if !validUsername(uname) {
			return "", "", ErrInvalidIdentifier
		}
		return uname, kindUsername, nil
	}
}

// normalizeAndValidate 把请求规范化并校验所有约束，返回入库形态 + 失败原因（仅供日志）。
//
// 返回的 reason 与外部 INVALID_REQUEST 文案解耦，仅供运维排查：
//
//	"no_identifier" / "multiple_identifiers" / "username_format" / "phone_format"
//	"email_format" / "password_length" / ""（成功时）
//
// 三个 identifier 字段的识别 + 规范化 + 格式校验**必须**通过 classifyIdentifier 完成；
// 注册和登录共用同一 helper，避免一边能注册另一边却查不到（v3 设计承诺）。
func normalizeAndValidate(req RegisterRequest) (*normalizedReq, string, error) {
	provided := 0
	type rawField struct {
		raw      string
		idType   string
		failCode string
	}
	fields := []rawField{
		{strings.TrimSpace(req.Username), "username", "username_format"},
		{strings.TrimSpace(req.Phone), "phone", "phone_format"},
		{strings.TrimSpace(req.Email), "email", "email_format"},
	}
	var picked rawField
	for _, f := range fields {
		if f.raw != "" {
			provided++
			picked = f
		}
	}
	if provided == 0 {
		return nil, "no_identifier", ErrInvalidIdentifier
	}
	if provided > 1 {
		return nil, "multiple_identifiers", ErrInvalidIdentifier
	}

	// 调用方明确给了哪一种 identifier；用 classifyIdentifier 走同一套规则。
	// 但 classifyIdentifier 是按内容自动识别 kind，对此处用力过度。这里复用其
	// 规范化 + 严格校验逻辑，并校验自动识别的结果与"调用方声明的 idType"一致——
	// 不一致说明调用方填错列（例如把 "alice@example.com" 填进 username 字段），
	// 走 INVALID_REQUEST 拒绝。
	norm, kind, err := classifyIdentifier(picked.raw)
	if err != nil {
		return nil, picked.failCode, ErrInvalidIdentifier
	}
	if string(kind) != picked.idType {
		// 例如 phone 字段填了 email 内容
		return nil, picked.failCode, ErrInvalidIdentifier
	}

	n := &normalizedReq{password: req.Password, idType: picked.idType}
	switch kind {
	case kindUsername:
		n.username = &norm
	case kindPhone:
		n.phone = &norm
	case kindEmail:
		n.email = &norm
	}

	pwLen := len(req.Password)
	if pwLen < pwdMinLen || pwLen > pwdMaxLen {
		return nil, "password_length", ErrInvalidPassword
	}

	return n, "", nil
}

// Login 验证 identifier + password，签发 access + refresh token。
//
// 错误统一为 ErrInvalidCredentials（用户不存在 / 密码错） / ErrInvalidIdentifier
// （identifier 格式错） / ErrInvalidPassword（密码长度违规）。
func (s *service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// password 长度先校验，避免对 service 整套流程的浪费。
	if len(req.Password) < pwdMinLen || len(req.Password) > pwdMaxLen {
		zap.L().Info("[user] login fail",
			zap.String("code", "INVALID_REQUEST"),
			zap.String("reason", "password_length"),
		)
		return nil, ErrInvalidPassword
	}

	norm, kind, err := classifyIdentifier(req.Identifier)
	if err != nil {
		zap.L().Info("[user] login fail",
			zap.String("code", "INVALID_REQUEST"),
			zap.String("reason", "identifier_format"),
		)
		return nil, ErrInvalidIdentifier
	}

	zap.L().Info("[user] login attempt",
		zap.String("identifier_type", string(kind)),
	)

	idType := store.IdentifierType(kind)
	user, err := s.store.FindByIdentifier(ctx, norm, idType)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			// 与"密码错"统一返回 INVALID_CREDENTIALS，缓解枚举。
			zap.L().Info("[user] login fail",
				zap.String("code", "INVALID_CREDENTIALS"),
				zap.String("identifier_type", string(kind)),
			)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		zap.L().Info("[user] login fail",
			zap.String("code", "INVALID_CREDENTIALS"),
			zap.String("identifier_type", string(kind)),
			zap.Uint64("user_id", user.ID),
		)
		return nil, ErrInvalidCredentials
	}

	// 状态校验：被禁用账号即使密码对也拒绝登录。
	// 与"密码错"统一返回 INVALID_CREDENTIALS，缓解枚举（攻击者无法通过返回码区分账号状态）。
	if user.Status != model.UserStatusActive {
		zap.L().Info("[user] login fail",
			zap.String("code", "INVALID_CREDENTIALS"),
			zap.String("reason", "user_disabled"),
			zap.Uint64("user_id", user.ID),
			zap.Int8("status", user.Status),
		)
		return nil, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, user.ID, string(kind))
}

// Refresh 用 refresh token 换新的 access + refresh（轮换）。
// 一切失败路径（签名 / 过期 / typ 不对 / Rotate mismatch）统一返回 ErrInvalidToken。
func (s *service) Refresh(ctx context.Context, req RefreshRequest) (*LoginResponse, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return nil, ErrInvalidIdentifier // handler 转 400 INVALID_REQUEST
	}
	claims, err := s.signer.Parse(req.RefreshToken, TokenRefresh)
	if err != nil {
		zap.L().Info("[user] refresh fail",
			zap.String("code", "INVALID_TOKEN"),
			zap.String("reason", "parse"),
		)
		return nil, ErrInvalidToken
	}
	uid, err := claims.SubjectAsUserID()
	if err != nil {
		zap.L().Warn("[user] refresh fail",
			zap.String("code", "INVALID_TOKEN"),
			zap.String("reason", "bad_subject"),
		)
		return nil, ErrInvalidToken
	}

	// 用户状态实时校验：禁用账号在 token 有效期内不能继续轮换 session。
	// 与"refresh 失败"其他路径统一返回 INVALID_TOKEN，不区分原因（缓解枚举）。
	user, err := s.store.FindByID(ctx, uid)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			zap.L().Info("[user] refresh fail",
				zap.String("code", "INVALID_TOKEN"),
				zap.String("reason", "user_not_found"),
				zap.Uint64("user_id", uid),
			)
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if user.Status != model.UserStatusActive {
		zap.L().Info("[user] refresh fail",
			zap.String("code", "INVALID_TOKEN"),
			zap.String("reason", "user_disabled"),
			zap.Uint64("user_id", uid),
			zap.Int8("status", user.Status),
		)
		return nil, ErrInvalidToken
	}

	// 旧 token 的 hash
	oldHash := HashRefreshToken(req.RefreshToken)
	oldJTI := claims.JTI

	// 生成新 jti / 新 refresh
	newJTI, err := NewJTI()
	if err != nil {
		return nil, err
	}
	newRefreshTok, refreshExp, err := s.signer.SignRefresh(uid, newJTI)
	if err != nil {
		return nil, err
	}
	newRefreshHash := HashRefreshToken(newRefreshTok)

	if err := s.refresh.Rotate(ctx, uid, oldJTI, oldHash, newJTI, newRefreshHash, time.Until(refreshExp)); err != nil {
		if errors.Is(err, ErrSessionMismatch) {
			zap.L().Info("[user] refresh fail",
				zap.String("code", "INVALID_TOKEN"),
				zap.String("reason", "session_mismatch"),
				zap.Uint64("user_id", uid),
				zap.String("old_jti_prefix", jtiPrefix(oldJTI)),
			)
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	accessTok, accessExp, err := s.signer.SignAccess(uid)
	if err != nil {
		return nil, err
	}

	zap.L().Info("[user] refresh success",
		zap.Uint64("user_id", uid),
		zap.String("old_jti_prefix", jtiPrefix(oldJTI)),
		zap.String("new_jti_prefix", jtiPrefix(newJTI)),
	)

	return &LoginResponse{
		UserID:           uid,
		AccessToken:      accessTok,
		RefreshToken:     newRefreshTok,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
		TokenType:        "Bearer",
	}, nil
}

// Logout 吊销 refresh token：校验 hash 后删 Redis key（Lua 原子）。
// 签名 / 过期 / typ 不对 → 401；hash 不匹配 / key 不存在 → 204 幂等成功。
func (s *service) Logout(ctx context.Context, req RefreshRequest) error {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return ErrInvalidIdentifier
	}
	claims, err := s.signer.Parse(req.RefreshToken, TokenRefresh)
	if err != nil {
		return ErrInvalidToken
	}
	uid, err := claims.SubjectAsUserID()
	if err != nil {
		return ErrInvalidToken
	}
	hash := HashRefreshToken(req.RefreshToken)
	// Revoke Lua 内部 compare-then-DEL；mismatch / not-found 返回 nil（幂等）。
	if err := s.refresh.Revoke(ctx, uid, claims.JTI, hash); err != nil {
		return err
	}
	zap.L().Info("[user] logout",
		zap.Uint64("user_id", uid),
		zap.String("jti_prefix", jtiPrefix(claims.JTI)),
	)
	return nil
}

// issueTokens 是 Login 成功后的公共出口：签新 jti / access / refresh，写 refresh 到 Redis。
func (s *service) issueTokens(ctx context.Context, uid uint64, idType string) (*LoginResponse, error) {
	jti, err := NewJTI()
	if err != nil {
		return nil, err
	}
	accessTok, accessExp, err := s.signer.SignAccess(uid)
	if err != nil {
		return nil, err
	}
	refreshTok, refreshExp, err := s.signer.SignRefresh(uid, jti)
	if err != nil {
		return nil, err
	}
	refreshHash := HashRefreshToken(refreshTok)
	if err := s.refresh.Save(ctx, uid, jti, refreshHash, time.Until(refreshExp)); err != nil {
		return nil, err
	}

	zap.L().Info("[user] login success",
		zap.Uint64("user_id", uid),
		zap.String("identifier_type", idType),
		zap.String("jti_prefix", jtiPrefix(jti)),
	)

	return &LoginResponse{
		UserID:           uid,
		AccessToken:      accessTok,
		RefreshToken:     refreshTok,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
		TokenType:        "Bearer",
	}, nil
}

// jtiPrefix 取 jti 前 8 字符用于日志（避免完整 jti 进日志便于排查滥用）。
func jtiPrefix(j string) string {
	if len(j) <= 8 {
		return j
	}
	return j[:8]
}
