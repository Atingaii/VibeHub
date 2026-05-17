package user

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"

	"github.com/vibeshop/vibeshop/internal/model"
	"github.com/vibeshop/vibeshop/internal/store"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// userStore 是 service 对 DAO 的最小依赖契约（便于单测 mock）。
type userStore interface {
	Create(ctx context.Context, u *model.User) error
}

// service 封装注册流程的业务逻辑。
type service struct {
	store userStore
}

func newService(s userStore) *service {
	return &service{store: s}
}

const (
	bcryptCost = 10
	pwdMinLen  = 8
	pwdMaxLen  = 72 // bcrypt 上限
)

var (
	usernameRE = regexp.MustCompile(`^[a-z0-9_-]{4,32}$`)
	phoneRE    = regexp.MustCompile(`^\d{6,20}$`)
	// emailRE 要求 local-part 至少一个字符 + '@' + domain（含至少一个 '.'，TLD 至少 2 字符）。
	// 配合 net/mail.ParseAddress 严格校验，拒绝 display-name 形式（"Foo <a@b>"）和无 TLD 形式（"a@b"）。
	emailRE = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

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

// normalizeAndValidate 把请求规范化并校验所有约束，返回入库形态 + 失败原因（仅供日志）。
//
// 返回的 reason 与外部 INVALID_REQUEST 文案解耦，仅供运维排查：
//   "no_identifier" / "multiple_identifiers" / "username_format" / "phone_format"
//   "email_format" / "password_length" / ""（成功时）
//
// 规范化（在校验之前做）：
//   - username / email：TrimSpace + ToLower
//   - phone：仅 TrimSpace
//
// 校验：
//   - 三标识有且仅有一个非空
//   - 各字段格式（username 正则 / phone 正则 / email 严格校验）
//   - password 长度 8-72
func normalizeAndValidate(req RegisterRequest) (*normalizedReq, string, error) {
	uname := strings.ToLower(strings.TrimSpace(req.Username))
	email := strings.ToLower(strings.TrimSpace(req.Email))
	phone := strings.TrimSpace(req.Phone)

	provided := 0
	idType := ""
	if uname != "" {
		provided++
		idType = "username"
	}
	if phone != "" {
		provided++
		idType = "phone"
	}
	if email != "" {
		provided++
		idType = "email"
	}
	if provided == 0 {
		return nil, "no_identifier", ErrInvalidIdentifier
	}
	if provided > 1 {
		return nil, "multiple_identifiers", ErrInvalidIdentifier
	}

	n := &normalizedReq{password: req.Password, idType: idType}
	switch idType {
	case "username":
		if !usernameRE.MatchString(uname) {
			return nil, "username_format", ErrInvalidIdentifier
		}
		n.username = &uname
	case "phone":
		if !phoneRE.MatchString(phone) {
			return nil, "phone_format", ErrInvalidIdentifier
		}
		n.phone = &phone
	case "email":
		// 严格校验：(a) regex 锁定 local@domain.tld 形态；(b) ParseAddress 拒绝畸形；
		// (c) addr.Name == "" 拒绝 "Foo <a@b.com>"；(d) addr.Address == email 防止 ParseAddress
		//     默默把 display-name 形式抽出 bare address 后绕过 (a)。
		if !emailRE.MatchString(email) {
			return nil, "email_format", ErrInvalidIdentifier
		}
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Name != "" || addr.Address != email {
			return nil, "email_format", ErrInvalidIdentifier
		}
		n.email = &email
	}

	pwLen := len(req.Password)
	if pwLen < pwdMinLen || pwLen > pwdMaxLen {
		return nil, "password_length", ErrInvalidPassword
	}

	return n, "", nil
}
