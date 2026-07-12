package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// StatusActive 是当前唯一允许创建或续期登录会话的账号状态。
const StatusActive = "ACTIVE"

var (
	ErrStudentNoExists     = errors.New("student number already exists")
	ErrAccountNotFound     = errors.New("account not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrInvalidAccessToken  = errors.New("invalid access token")
	ErrInvalidInput        = errors.New("invalid input")
)

// Account 表示认证领域所需的最小账号信息。
// PasswordHash 只能参与服务端校验，禁止通过 HTTP 响应返回或写入日志。
type Account struct {
	ID, StudentNo, Name, PasswordHash, Status string
	CreatedAt                                 time.Time
}

// RefreshSession 只保存浏览器刷新令牌的哈希值，避免数据库内容泄露后
// 攻击者可以直接利用其中的数据接管仍然有效的会话。
type RefreshSession struct {
	ID, AccountID, TokenHash string
	ExpiresAt                time.Time
	RevokedAt                *time.Time
}

// AccountRepository 定义认证领域依赖的账号持久化能力，使领域逻辑不依赖 PostgreSQL 实现。
type AccountRepository interface {
	Create(context.Context, Account) error
	FindByStudentNo(context.Context, string) (Account, error)
	FindByID(context.Context, string) (Account, error)
}

// SessionRepository 必须以原子方式消费刷新会话，确保刷新令牌只能成功使用一次。
type SessionRepository interface {
	Save(context.Context, RefreshSession) error
	Consume(context.Context, string, time.Time) (RefreshSession, error)
	Revoke(context.Context, string, time.Time) error
}

// RegisterInput 是当前阶段的最小注册契约。
// 部门、角色和审核字段需等待第 0 阶段业务规则确认，因此暂不提前写入该契约。
type RegisterInput struct{ StudentNo, Name, Password string }

// Tokens 包含短期访问令牌和每次续期都会轮换的刷新令牌。
type Tokens struct {
	AccessToken, RefreshToken         string
	AccessExpiresAt, RefreshExpiresAt time.Time
}

// Service 封装认证规则，不依赖具体 HTTP 框架和 PostgreSQL 实现。
type Service struct {
	accounts              AccountRepository
	sessions              SessionRepository
	secret                []byte
	accessTTL, refreshTTL time.Duration
	now                   func() time.Time
}

// NewService 使用显式的令牌有效期创建认证服务，便于不同环境配置和测试。
func NewService(a AccountRepository, s SessionRepository, secret []byte, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{accounts: a, sessions: s, secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}
}

// Register 在当前限定范围内创建可直接登录的账号。
// 注册审核规则确认后，应由审核流程替换这里的直接激活策略。
func (s *Service) Register(ctx context.Context, in RegisterInput) (Account, error) {
	in.StudentNo, in.Name = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name)
	if in.StudentNo == "" || in.Name == "" || len(in.Password) < 8 {
		return Account{}, ErrInvalidInput
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, err
	}
	a := Account{ID: uuid.NewString(), StudentNo: in.StudentNo, Name: in.Name, PasswordHash: string(hash), Status: StatusActive, CreatedAt: s.now().UTC()}
	if err := s.accounts.Create(ctx, a); err != nil {
		return Account{}, err
	}
	return a, nil
}

// Login 对账号不存在、密码错误和账号不可用统一返回凭证错误，
// 避免通过错误差异探测系统中是否存在某个学号。
func (s *Service) Login(ctx context.Context, studentNo, password string) (Tokens, error) {
	a, err := s.accounts.FindByStudentNo(ctx, strings.TrimSpace(studentNo))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil || a.Status != StatusActive {
		return Tokens{}, ErrInvalidCredentials
	}
	return s.issueTokens(ctx, a)
}

// Refresh 先消费旧令牌再签发新令牌；即使旧令牌曾被复制，后续重放也会失败。
func (s *Service) Refresh(ctx context.Context, token string) (Tokens, error) {
	now := s.now().UTC()
	session, err := s.sessions.Consume(ctx, tokenHash(token), now)
	if err != nil {
		return Tokens{}, ErrInvalidRefreshToken
	}
	a, err := s.accounts.FindByID(ctx, session.AccountID)
	if err != nil || a.Status != StatusActive {
		return Tokens{}, ErrInvalidRefreshToken
	}
	return s.issueTokens(ctx, a)
}

// Logout 保持幂等，客户端在网络重试后仍可安全清理本地登录状态。
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Revoke(ctx, tokenHash(token), s.now().UTC())
}

// AuthenticateAccessToken 同时验证 JWT 和数据库中的实时账号状态。
// 因此账号被禁用后，即使旧 JWT 尚未过期，也不能继续访问受保护接口。
func (s *Service) AuthenticateAccessToken(ctx context.Context, raw string) (Account, error) {
	claims := jwt.MapClaims{}
	t, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidAccessToken
		}
		return s.secret, nil
	})
	if err != nil || !t.Valid {
		return Account{}, ErrInvalidAccessToken
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return Account{}, ErrInvalidAccessToken
	}
	a, err := s.accounts.FindByID(ctx, sub)
	if err != nil || a.Status != StatusActive {
		return Account{}, ErrInvalidAccessToken
	}
	return a, nil
}

func (s *Service) issueTokens(ctx context.Context, a Account) (Tokens, error) {
	now := s.now().UTC()
	accessExp, refreshExp := now.Add(s.accessTTL), now.Add(s.refreshTTL)
	claims := jwt.RegisteredClaims{Subject: a.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExp), Issuer: "four-check-management-system"}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return Tokens{}, err
	}
	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		return Tokens{}, err
	}
	refresh := base64.RawURLEncoding.EncodeToString(refreshBytes)
	if err := s.sessions.Save(ctx, RefreshSession{ID: uuid.NewString(), AccountID: a.ID, TokenHash: tokenHash(refresh), ExpiresAt: refreshExp}); err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, AccessExpiresAt: accessExp, RefreshExpiresAt: refreshExp}, nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}
