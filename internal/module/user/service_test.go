package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vibeshop/vibeshop/internal/model"
	"github.com/vibeshop/vibeshop/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// fakeStore 是 userStore 的内存实现，用于 service 层测试。
type fakeStore struct {
	created     []*model.User
	returnTaken bool
	failNext    error
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

func newServiceForTest() (*service, *fakeStore) {
	fs := &fakeStore{}
	return newService(fs), fs
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
