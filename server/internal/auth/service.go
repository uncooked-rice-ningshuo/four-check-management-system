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

	"four-check-management-system/server/internal/audit"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	StatusPending  = "PENDING"
	StatusActive   = "ACTIVE"
	StatusDisabled = "DISABLED"
	StatusRejected = "REJECTED"

	AccountKindSystem = "SYSTEM"
	AccountKindPerson = "PERSON"

	SystemRoleSuperAdmin = "SUPER_ADMIN"
	SystemRoleAdmin      = "ADMIN"
	SystemRoleMember     = "MEMBER"

	ReviewApprove = "APPROVE"
	ReviewReject  = "REJECT"
)

var (
	ErrLoginNameExists     = errors.New("login name already exists")
	ErrStudentNoExists     = ErrLoginNameExists
	ErrAccountNotFound     = errors.New("account not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrInvalidAccessToken  = errors.New("invalid access token")
	ErrInvalidInput        = errors.New("invalid input")
	ErrForbidden           = errors.New("forbidden")
	ErrInvalidTransition   = errors.New("invalid account status transition")
)

// Account 是认证与管理端使用的安全账号领域模型。PasswordHash 只能在服务端参与校验。
type Account struct {
	ID                    string
	LoginName             string
	StudentNo             string
	Name                  string
	PasswordHash          string
	Status                string
	AccountKind           string
	IsSuperAdmin          bool
	SystemRole            string
	MustChangePassword    bool
	SessionVersion        int
	PrimaryDepartmentID   string
	RequestedDepartmentID string
	RequestedOrgRoleID    string
	CreatedAt             time.Time
}

type RefreshSession struct {
	ID, AccountID, TokenHash string
	ExpiresAt                time.Time
	RevokedAt                *time.Time
}

type accessClaims struct {
	SessionVersion int `json:"ver"`
	jwt.RegisteredClaims
}

// AccountRepository 把账号状态迁移和审核事务交给持久化层原子执行。
type AccountRepository interface {
	Create(context.Context, Account, audit.Mutation) error
	FindByLoginName(context.Context, string) (Account, error)
	FindByID(context.Context, string) (Account, error)
	UpdatePassword(context.Context, string, string, bool, audit.Mutation) error
	ListPending(context.Context) ([]Account, error)
	ReviewRegistration(context.Context, string, string, string, string, time.Time, audit.Mutation) (Account, error)
	UpdateAccount(context.Context, Account, audit.Mutation) (Account, error)
	SetStatus(context.Context, string, string, audit.Mutation) (Account, error)
}

type SessionRepository interface {
	Save(context.Context, RefreshSession) error
	Consume(context.Context, string, time.Time) (RefreshSession, error)
	Revoke(context.Context, string, time.Time) error
	RevokeAll(context.Context, string, time.Time) error
}

type RegisterInput struct {
	StudentNo, Name, Password                 string
	RequestedDepartmentID, RequestedOrgRoleID string
}

type AdminCreateUserInput struct {
	StudentNo, Name, TemporaryPassword, SystemRole string
}

type AdminUpdateUserInput struct {
	StudentNo, Name, SystemRole string
}

type Tokens struct {
	AccessToken, RefreshToken         string
	AccessExpiresAt, RefreshExpiresAt time.Time
}

type Service struct {
	accounts              AccountRepository
	sessions              SessionRepository
	secret                []byte
	accessTTL, refreshTTL time.Duration
	now                   func() time.Time
}

func NewService(a AccountRepository, s SessionRepository, secret []byte, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{accounts: a, sessions: s, secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}
}

// BootstrapAdmin 幂等创建不依赖学号的系统管理员；已有管理员不会被启动配置覆盖。
func (s *Service) BootstrapAdmin(ctx context.Context, loginName, name, password string, metadata ...audit.Metadata) (Account, error) {
	loginName, name = strings.TrimSpace(loginName), strings.TrimSpace(name)
	if loginName == "" || name == "" || len(password) < 12 {
		return Account{}, ErrInvalidInput
	}
	if existing, err := s.accounts.FindByLoginName(ctx, loginName); err == nil {
		if !existing.IsSuperAdmin || existing.AccountKind != AccountKindSystem {
			return Account{}, ErrLoginNameExists
		}
		return existing, nil
	} else if !errors.Is(err, ErrAccountNotFound) {
		return Account{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Account{}, err
	}
	a := Account{
		ID: uuid.NewString(), LoginName: loginName, Name: name, PasswordHash: hash,
		Status: StatusActive, AccountKind: AccountKindSystem, IsSuperAdmin: true,
		SystemRole: SystemRoleSuperAdmin, MustChangePassword: true, CreatedAt: s.now().UTC(),
	}
	mutation := accountMutation(metadata, "", audit.ActionAccountCreated, a.ID, nil, accountAuditState(a), "")
	if err := s.accounts.Create(ctx, a, mutation); err != nil {
		if errors.Is(err, ErrLoginNameExists) {
			return s.accounts.FindByLoginName(ctx, loginName)
		}
		return Account{}, err
	}
	return a, nil
}

// Register 创建待审核个人账号，登录名固定等于学号，避免用户自行选择可混淆的账号名。
func (s *Service) Register(ctx context.Context, in RegisterInput, metadata ...audit.Metadata) (Account, error) {
	in.StudentNo, in.Name = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name)
	if in.StudentNo == "" || in.Name == "" || len(in.Password) < 8 || in.RequestedDepartmentID == "" || in.RequestedOrgRoleID == "" {
		return Account{}, ErrInvalidInput
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return Account{}, err
	}
	a := Account{
		ID: uuid.NewString(), LoginName: in.StudentNo, StudentNo: in.StudentNo, Name: in.Name,
		PasswordHash: hash, Status: StatusPending, AccountKind: AccountKindPerson, SystemRole: SystemRoleMember,
		RequestedDepartmentID: in.RequestedDepartmentID, RequestedOrgRoleID: in.RequestedOrgRoleID,
		CreatedAt: s.now().UTC(),
	}
	mutation := accountMutation(metadata, "", audit.ActionAccountCreated, a.ID, nil, accountAuditState(a), "")
	if err := s.accounts.Create(ctx, a, mutation); err != nil {
		return Account{}, err
	}
	return a, nil
}

// AdminCreateUser 创建可立即启用的个人账号，并强制首次登录修改临时密码。
func (s *Service) AdminCreateUser(ctx context.Context, actor Account, in AdminCreateUserInput, metadata ...audit.Metadata) (Account, error) {
	if !actor.IsSuperAdmin {
		return Account{}, ErrForbidden
	}
	in.StudentNo, in.Name, in.SystemRole = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name), strings.TrimSpace(in.SystemRole)
	if in.StudentNo == "" || in.Name == "" || len(in.TemporaryPassword) < 12 || !validManagedSystemRole(in.SystemRole) {
		return Account{}, ErrInvalidInput
	}
	hash, err := hashPassword(in.TemporaryPassword)
	if err != nil {
		return Account{}, err
	}
	account := Account{ID: uuid.NewString(), LoginName: in.StudentNo, StudentNo: in.StudentNo, Name: in.Name, PasswordHash: hash,
		Status: StatusActive, AccountKind: AccountKindPerson, SystemRole: in.SystemRole, MustChangePassword: true, CreatedAt: s.now().UTC()}
	mutation := accountMutation(metadata, actor.ID, audit.ActionAccountCreated, account.ID, nil, accountAuditState(account), "")
	if err := s.accounts.Create(ctx, account, mutation); err != nil {
		return Account{}, err
	}
	return account, nil
}

// AdminUpdateUser 更新普通账号身份信息；系统管理员账号不能通过普通用户接口改写。
func (s *Service) AdminUpdateUser(ctx context.Context, actor Account, accountID string, in AdminUpdateUserInput, metadata ...audit.Metadata) (Account, error) {
	if !actor.IsSuperAdmin {
		return Account{}, ErrForbidden
	}
	target, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return Account{}, err
	}
	if target.IsSuperAdmin || target.AccountKind == AccountKindSystem {
		return Account{}, ErrForbidden
	}
	in.StudentNo, in.Name, in.SystemRole = strings.TrimSpace(in.StudentNo), strings.TrimSpace(in.Name), strings.TrimSpace(in.SystemRole)
	if in.StudentNo == "" || in.Name == "" || !validManagedSystemRole(in.SystemRole) {
		return Account{}, ErrInvalidInput
	}
	before := accountAuditState(target)
	target.StudentNo, target.LoginName, target.Name, target.SystemRole = in.StudentNo, in.StudentNo, in.Name, in.SystemRole
	mutation := accountMutation(metadata, actor.ID, audit.ActionAccountUpdated, target.ID, before, accountAuditState(target), "")
	updated, err := s.accounts.UpdateAccount(ctx, target, mutation)
	if err != nil {
		return Account{}, err
	}
	if err := s.sessions.RevokeAll(ctx, accountID, s.now().UTC()); err != nil {
		return Account{}, err
	}
	return updated, nil
}

func (s *Service) AdminSetUserStatus(ctx context.Context, actor Account, accountID, status string, metadata ...audit.Metadata) (Account, error) {
	if !actor.IsSuperAdmin {
		return Account{}, ErrForbidden
	}
	target, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return Account{}, err
	}
	if target.IsSuperAdmin || (status != StatusActive && status != StatusDisabled) {
		return Account{}, ErrForbidden
	}
	mutation := accountMutation(metadata, actor.ID, audit.ActionAccountStatusChanged, target.ID,
		map[string]any{"status": target.Status}, map[string]any{"status": status}, "")
	updated, err := s.accounts.SetStatus(ctx, accountID, status, mutation)
	if err != nil {
		return Account{}, err
	}
	if err := s.sessions.RevokeAll(ctx, accountID, s.now().UTC()); err != nil {
		return Account{}, err
	}
	return updated, nil
}

func (s *Service) AdminResetPassword(ctx context.Context, actor Account, accountID, temporaryPassword string, metadata ...audit.Metadata) error {
	if !actor.IsSuperAdmin {
		return ErrForbidden
	}
	target, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if target.IsSuperAdmin || len(temporaryPassword) < 12 {
		return ErrForbidden
	}
	hash, err := hashPassword(temporaryPassword)
	if err != nil {
		return err
	}
	mutation := accountMutation(metadata, actor.ID, audit.ActionAccountPasswordReset, target.ID,
		map[string]any{"mustChangePassword": target.MustChangePassword}, map[string]any{"mustChangePassword": true}, "")
	if err := s.accounts.UpdatePassword(ctx, accountID, hash, true, mutation); err != nil {
		return err
	}
	return s.sessions.RevokeAll(ctx, accountID, s.now().UTC())
}

func validManagedSystemRole(role string) bool {
	return role == SystemRoleAdmin || role == SystemRoleMember
}

func (s *Service) Login(ctx context.Context, loginName, password string) (Tokens, error) {
	a, err := s.accounts.FindByLoginName(ctx, strings.TrimSpace(loginName))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil || a.Status != StatusActive {
		return Tokens{}, ErrInvalidCredentials
	}
	return s.issueTokens(ctx, a)
}

// ChangePassword 校验当前密码并撤销全部刷新会话，防止旧会话在改密后继续续期。
func (s *Service) ChangePassword(ctx context.Context, actor Account, currentPassword, newPassword string, metadata ...audit.Metadata) error {
	if len(newPassword) < 12 || currentPassword == newPassword {
		return ErrInvalidInput
	}
	current, err := s.accounts.FindByID(ctx, actor.ID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(current.PasswordHash), []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	mutation := accountMutation(metadata, actor.ID, audit.ActionAccountPasswordReset, current.ID,
		map[string]any{"mustChangePassword": current.MustChangePassword}, map[string]any{"mustChangePassword": false}, "")
	if err := s.accounts.UpdatePassword(ctx, current.ID, hash, false, mutation); err != nil {
		return err
	}
	return s.sessions.RevokeAll(ctx, current.ID, s.now().UTC())
}

func (s *Service) ListPendingRegistrations(ctx context.Context, actor Account) ([]Account, error) {
	if !actor.IsSuperAdmin {
		return nil, ErrForbidden
	}
	return s.accounts.ListPending(ctx)
}

func (s *Service) ReviewRegistration(ctx context.Context, actor Account, accountID, decision, reason string, metadata ...audit.Metadata) (Account, error) {
	if !actor.IsSuperAdmin {
		return Account{}, ErrForbidden
	}
	if decision != ReviewApprove && decision != ReviewReject {
		return Account{}, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	if decision == ReviewReject && reason == "" {
		return Account{}, ErrInvalidInput
	}
	newStatus := StatusRejected
	if decision == ReviewApprove {
		newStatus = StatusActive
	}
	mutation := accountMutation(metadata, actor.ID, audit.ActionRegistrationReviewed, accountID,
		map[string]any{"status": StatusPending}, map[string]any{"status": newStatus, "decision": decision}, reason)
	return s.accounts.ReviewRegistration(ctx, accountID, actor.ID, decision, reason, s.now().UTC(), mutation)
}

func accountMutation(metadata []audit.Metadata, actorID string, action audit.Action, resourceID string, before, after any, reason string) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	if request.ActorID == "" {
		request.ActorID = actorID
	}
	if request.Reason == "" {
		request.Reason = reason
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: action, ResourceType: "account", ResourceID: resourceID, Before: before, After: after,
	}}
}

func accountAuditState(account Account) map[string]any {
	return map[string]any{
		"id": account.ID, "loginName": account.LoginName, "studentNo": account.StudentNo, "name": account.Name,
		"status": account.Status, "accountKind": account.AccountKind, "systemRole": account.SystemRole,
		"mustChangePassword": account.MustChangePassword, "primaryDepartmentId": account.PrimaryDepartmentID,
	}
}

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

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Revoke(ctx, tokenHash(token), s.now().UTC())
}

func (s *Service) AuthenticateAccessToken(ctx context.Context, raw string) (Account, error) {
	claims := accessClaims{}
	t, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
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
	if err != nil || a.Status != StatusActive || claims.SessionVersion != a.SessionVersion {
		return Account{}, ErrInvalidAccessToken
	}
	return a, nil
}

func (s *Service) issueTokens(ctx context.Context, a Account) (Tokens, error) {
	now := s.now().UTC()
	accessExp, refreshExp := now.Add(s.accessTTL), now.Add(s.refreshTTL)
	claims := accessClaims{
		SessionVersion: a.SessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: a.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(accessExp), Issuer: "four-check-management-system",
		},
	}
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

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum[:])
}
