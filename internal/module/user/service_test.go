package user

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/vibeshop/vibeshop/internal/model"
	"github.com/vibeshop/vibeshop/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// fakeStore 是 userStore 的内存实现，用于 service 层测试。
type fakeStore struct {
	created     []*model.User
	returnTaken bool
	failNext    error

	// 1.2 登录用：findResult 优先返回；nil 时返回 ErrUserNotFound
	findResult *model.User
	findErr    error

	// FindByID 桩；nil 时退回 findResult
	findByIDResult *model.User
	findByIDErr    error
}

func (f *fakeStore) Create(_ context.Context, u *model.User) error {
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	if f.returnTaken {
		return store.ErrIdentifierTaken
	}
	u.ID = uint64(len(f.created)) + 1
	f.created = append(f.created, u)
	return nil
}

func (f *fakeStore) FindByIdentifier(_ context.Context, _ string, _ store.IdentifierType) (*model.User, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	if f.findResult != nil {
		return f.findResult, nil
	}
	return nil, store.ErrUserNotFound
}

// FindByID：默认返回 findResult（同一个用户对象）；测试可注入 findByIDErr / findByIDResult。
func (f *fakeStore) FindByID(_ context.Context, _ uint64) (*model.User, error) {
	if f.findByIDErr != nil {
		return nil, f.findByIDErr
	}
	if f.findByIDResult != nil {
		return f.findByIDResult, nil
	}
	if f.findResult != nil {
		return f.findResult, nil
	}
	return nil, store.ErrUserNotFound
}

func newServiceForTest() (*service, *fakeStore) {
	fs := &fakeStore{}
	return newService(fs, nil, nil), fs
}

func TestRegister_SuccessByUsername(t *testing.T) {
	svc, fs := newServiceForTest()
	resp, err := svc.Register(context.Background(), RegisterRequest{
		Username: "John_Doe",
		Password: "Strong#1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Username == nil || *resp.Username != "john_doe" {
		t.Fatalf("expected username normalized to 'john_doe', got %v", resp.Username)
	}
	if resp.Phone != nil || resp.Email != nil {
		t.Fatalf("phone/email should be nil, got phone=%v email=%v", resp.Phone, resp.Email)
	}
	if len(fs.created) != 1 {
		t.Fatalf("expected 1 record, got %d", len(fs.created))
	}
	if fs.created[0].Status != model.UserStatusActive {
		t.Fatalf("expected status=active, got %d", fs.created[0].Status)
	}
}

func TestRegister_SuccessByPhone(t *testing.T) {
	svc, fs := newServiceForTest()
	resp, err := svc.Register(context.Background(), RegisterRequest{
		Phone:    " 13800138000 ",
		Password: "Strong#1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Phone == nil || *resp.Phone != "13800138000" {
		t.Fatalf("expected phone trimmed to '13800138000', got %v", resp.Phone)
	}
	if fs.created[0].Username != nil || fs.created[0].Email != nil {
		t.Fatalf("username/email should be nil")
	}
}

func TestRegister_SuccessByEmail(t *testing.T) {
	svc, _ := newServiceForTest()
	resp, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "  Foo@Example.COM ",
		Password: "Strong#1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Email == nil || *resp.Email != "foo@example.com" {
		t.Fatalf("expected email normalized to 'foo@example.com', got %v", resp.Email)
	}
}

func TestRegister_PasswordHashed(t *testing.T) {
	svc, fs := newServiceForTest()
	const plain = "Strong#1"
	if _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Password: plain,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hash := fs.created[0].PasswordHash
	if hash == plain {
		t.Fatalf("password stored in plain text")
	}
	if !strings.HasPrefix(hash, "$2a$10$") && !strings.HasPrefix(hash, "$2b$10$") {
		t.Fatalf("expected bcrypt cost-10 prefix, got %q", hash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		t.Fatalf("bcrypt verify failed: %v", err)
	}
}

func TestRegister_RejectMultipleIdentifiers(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Email:    "a@b.com",
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestRegister_RejectNoIdentifier(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestRegister_RejectShortPassword(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "short",
	})
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestRegister_RejectLongPassword(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Password: strings.Repeat("a", 73),
	})
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestRegister_RejectInvalidUsernameChars(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice!!", // 含非法字符
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestRegister_RejectShortUsername(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "abc", // 长度 3，下界 4
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestRegister_RejectInvalidEmail(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "not-an-email",
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

// TestRegister_RejectEmailDisplayName 防御 net/mail.ParseAddress 默默接受 "Foo <a@b.com>"
// 形式后只取 bare address 入库，导致同一 mailbox 两次注册成功的漏洞（Codex BLOCKER）。
func TestRegister_RejectEmailDisplayName(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "Foo <foo@example.com>",
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier for display-name email, got %v", err)
	}
}

// TestRegister_RejectEmailNoTLD 防御 ParseAddress 接受 "user@host" 这类无 TLD 邮箱。
func TestRegister_RejectEmailNoTLD(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "foo@example",
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier for no-TLD email, got %v", err)
	}
}

func TestRegister_RejectInvalidPhone(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Phone:    "12-34-56", // 含非数字
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestRegister_StoreDupKey(t *testing.T) {
	svc, fs := newServiceForTest()
	fs.returnTaken = true
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrIdentifierTaken) {
		t.Fatalf("expected ErrIdentifierTaken, got %v", err)
	}
}

func TestRegister_StoreOtherErrorPropagates(t *testing.T) {
	svc, fs := newServiceForTest()
	custom := errors.New("db down")
	fs.failNext = custom
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "alice",
		Password: "Strong#1",
	})
	if err == nil || errors.Is(err, ErrIdentifierTaken) || errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected raw error to propagate, got %v", err)
	}
}

// === 1.2 Login / Refresh / Logout ===

// newLoginServiceForTest 构造一个 Login/Refresh/Logout 可用的 service：
// - fakeStore（可注入 findResult / findErr）
// - 真 JWTSigner（HS256，固定 secret）
// - 真 RefreshStore + miniredis（模拟 PoolSession）
func newLoginServiceForTest(t *testing.T) (*service, *fakeStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	signer, err := NewJWTSigner([]byte(strings.Repeat("a", MinJWTSecretBytes)),
		2*time.Hour, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	fs := &fakeStore{}
	return newService(fs, signer, NewRedisRefreshStore(rdb)), fs, mr
}

// makeUser 用 bcrypt 哈希构造一个测试 User。
func makeUser(t *testing.T, id uint64, idType identifierKind, normValue, password string) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	u := &model.User{
		ID:           id,
		PasswordHash: string(hash),
		Status:       model.UserStatusActive,
	}
	switch idType {
	case kindUsername:
		u.Username = &normValue
	case kindPhone:
		u.Phone = &normValue
	case kindEmail:
		u.Email = &normValue
	}
	return u
}

func TestLogin_SuccessByUsername(t *testing.T) {
	svc, fs, mr := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 42, kindUsername, "alice", "Strong#1")
	resp, err := svc.Login(context.Background(), LoginRequest{Identifier: "Alice", Password: "Strong#1"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.UserID != 42 || resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if resp.AccessExpiresAt.Before(time.Now()) || resp.RefreshExpiresAt.Before(resp.AccessExpiresAt) {
		t.Fatalf("expiration order wrong: %v / %v", resp.AccessExpiresAt, resp.RefreshExpiresAt)
	}
	// 验证 refresh 已落 Redis
	keys := mr.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 refresh key in Redis, got %d", len(keys))
	}
}

func TestLogin_SuccessByPhone(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindPhone, "13800138000", "Strong#1")
	if _, err := svc.Login(context.Background(), LoginRequest{Identifier: "13800138000", Password: "Strong#1"}); err != nil {
		t.Fatalf("Login by phone: %v", err)
	}
}

func TestLogin_SuccessByEmail(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindEmail, "alice@example.com", "Strong#1")
	if _, err := svc.Login(context.Background(), LoginRequest{Identifier: " Alice@Example.COM ", Password: "Strong#1"}); err != nil {
		t.Fatalf("Login by email: %v", err)
	}
}

func TestLogin_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	svc, _, _ := newLoginServiceForTest(t)
	// fs.findResult 留 nil → fakeStore 默认返回 ErrUserNotFound
	_, err := svc.Login(context.Background(), LoginRequest{Identifier: "nobody", Password: "Strong#1"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindUsername, "alice", "Right#1!")
	_, err := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Wrong#1!"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_BadIdentifierFormat(t *testing.T) {
	svc, _, _ := newLoginServiceForTest(t)
	_, err := svc.Login(context.Background(), LoginRequest{Identifier: "!!", Password: "Strong#1"})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestLogin_ShortPassword(t *testing.T) {
	svc, _, _ := newLoginServiceForTest(t)
	_, err := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "x"})
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestRefresh_RotatesAndOldTokenInvalid(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 7, kindUsername, "alice", "Strong#1")
	first, err := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	second, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh did not rotate")
	}

	// 旧 refresh 立即失效
	if _, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected old refresh -> ErrInvalidToken, got %v", err)
	}
	// 新 refresh 仍能再轮换
	if _, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: second.RefreshToken}); err != nil {
		t.Fatalf("new refresh should still work: %v", err)
	}
}

func TestRefresh_RejectsAccessToken(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindUsername, "alice", "Strong#1")
	resp, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	// 拿 access 当 refresh 用 → 必须 401
	if _, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: resp.AccessToken}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected access-as-refresh -> ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_EmptyToken(t *testing.T) {
	svc, _, _ := newLoginServiceForTest(t)
	_, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: ""})
	if !errors.Is(err, ErrInvalidIdentifier) { // handler 会转 400
		t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
	}
}

func TestLogout_SuccessThenRefreshFails(t *testing.T) {
	svc, fs, mr := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 9, kindUsername, "alice", "Strong#1")
	first, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	if err := svc.Logout(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if len(mr.Keys()) != 0 {
		t.Fatal("expected refresh key removed after logout")
	}
	// 旧 refresh 不可再用
	if _, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken after logout, got %v", err)
	}
}

func TestLogout_Idempotent(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindUsername, "alice", "Strong#1")
	first, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	_ = svc.Logout(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken})
	// 第二次：仍 nil（幂等），不返回错误
	if err := svc.Logout(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken}); err != nil {
		t.Fatalf("Logout should be idempotent, got %v", err)
	}
}

// 退出门：已轮换的旧 refresh 调 logout 不影响新 jti 对应的 key。
func TestLogout_DoesNotKickRotatedSession(t *testing.T) {
	svc, fs, mr := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindUsername, "alice", "Strong#1")
	first, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	second, _ := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken})
	// 旧 refresh 调 logout —— Revoke 走 key-not-found 幂等
	if err := svc.Logout(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	// 新 jti 的 key 仍在 Redis
	if len(mr.Keys()) != 1 {
		t.Fatalf("new session key should remain; got %d keys", len(mr.Keys()))
	}
	// 新 refresh 仍可用
	if _, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: second.RefreshToken}); err != nil {
		t.Fatalf("new refresh should still work: %v", err)
	}
}

func TestLogout_RejectsAccessToken(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	fs.findResult = makeUser(t, 1, kindUsername, "alice", "Strong#1")
	resp, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	if err := svc.Logout(context.Background(), RefreshRequest{RefreshToken: resp.AccessToken}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on access-as-refresh, got %v", err)
	}
}

// classifyIdentifier 单测：和 1.1 同口径——同样的输入识别为同样的 kind。
func TestClassifyIdentifier_Username(t *testing.T) {
	norm, kind, err := classifyIdentifier("Alice_42")
	if err != nil || kind != kindUsername || norm != "alice_42" {
		t.Fatalf("got (%q, %q, %v)", norm, kind, err)
	}
}

func TestClassifyIdentifier_Phone(t *testing.T) {
	norm, kind, err := classifyIdentifier(" 13800138000 ")
	if err != nil || kind != kindPhone || norm != "13800138000" {
		t.Fatalf("got (%q, %q, %v)", norm, kind, err)
	}
}

func TestClassifyIdentifier_Email(t *testing.T) {
	norm, kind, err := classifyIdentifier(" Alice@Example.COM ")
	if err != nil || kind != kindEmail || norm != "alice@example.com" {
		t.Fatalf("got (%q, %q, %v)", norm, kind, err)
	}
}

func TestClassifyIdentifier_RejectsDisplayNameEmail(t *testing.T) {
	if _, _, err := classifyIdentifier("Foo <foo@example.com>"); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestClassifyIdentifier_RejectsEmpty(t *testing.T) {
	if _, _, err := classifyIdentifier("   "); err == nil {
		t.Fatal("expected rejection for blank")
	}
}

// === 1.2 disabled user 拦截 ===

func TestLogin_DisabledUser_ReturnsInvalidCredentials(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	user := makeUser(t, 1, kindUsername, "alice", "Strong#1")
	user.Status = model.UserStatusDisabled
	fs.findResult = user
	_, err := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for disabled user, got %v", err)
	}
}

func TestRefresh_DisabledUser_ReturnsInvalidToken(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	user := makeUser(t, 1, kindUsername, "alice", "Strong#1")
	fs.findResult = user
	first, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})

	// 模拟：在 refresh 时账号已被禁用
	disabled := *user
	disabled.Status = model.UserStatusDisabled
	fs.findByIDResult = &disabled

	_, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for disabled user on refresh, got %v", err)
	}
}

func TestRefresh_UserDeleted_ReturnsInvalidToken(t *testing.T) {
	svc, fs, _ := newLoginServiceForTest(t)
	user := makeUser(t, 1, kindUsername, "alice", "Strong#1")
	fs.findResult = user
	first, _ := svc.Login(context.Background(), LoginRequest{Identifier: "alice", Password: "Strong#1"})

	// 模拟：用户记录被删（FindByID 返回 ErrUserNotFound）
	fs.findByIDErr = store.ErrUserNotFound

	_, err := svc.Refresh(context.Background(), RefreshRequest{RefreshToken: first.RefreshToken})
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for deleted user on refresh, got %v", err)
	}
}

// 1.2 收紧的产品规则：纯数字 username 不可注册（避免与 phone 数值空间撞值）。
func TestRegister_RejectPureDigitUsername(t *testing.T) {
	svc, _ := newServiceForTest()
	_, err := svc.Register(context.Background(), RegisterRequest{
		Username: "123456", // 6 位纯数字，会与 phone 识别撞
		Password: "Strong#1",
	})
	if !errors.Is(err, ErrInvalidIdentifier) {
		t.Fatalf("expected ErrInvalidIdentifier for pure-digit username, got %v", err)
	}
}

func TestClassifyIdentifier_PureDigitGoesToPhone(t *testing.T) {
	// 6+ 位纯数字应识别为 phone，不会被当 username 处理（避免歧义）。
	norm, kind, err := classifyIdentifier("123456")
	if err != nil || kind != kindPhone || norm != "123456" {
		t.Fatalf("got (%q, %q, %v); expected phone classification", norm, kind, err)
	}
}

func TestClassifyIdentifier_MixedDigitLetterIsUsername(t *testing.T) {
	// 含字母 → username 分支
	norm, kind, err := classifyIdentifier("user123")
	if err != nil || kind != kindUsername || norm != "user123" {
		t.Fatalf("got (%q, %q, %v); expected username", norm, kind, err)
	}
}
