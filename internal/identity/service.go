package identity

import (
	"context"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/security"
)

// EmailMode controls verification / recovery delivery behavior.
type EmailMode string

const (
	// EmailModeNone stores tokens but does not deliver or expose them.
	EmailModeNone EmailMode = "none"
	// EmailModeDev exposes single-use tokens in API responses (non-production only).
	EmailModeDev EmailMode = "dev"
)

// Config holds identity service runtime settings.
type Config struct {
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	VerifyTTL        time.Duration
	PasswordResetTTL time.Duration
	EmailMode        EmailMode
	AppEnv           string
	ReuseGrace       time.Duration
}

// Service implements WPass identity application use cases.
type Service struct {
	store  *Store
	access *AccessTokenManager
	cfg    Config
	log    *slog.Logger
	now    func() time.Time
}

// NewService constructs the identity application service.
func NewService(store *Store, access *AccessTokenManager, cfg Config, log *slog.Logger) *Service {
	if cfg.ReuseGrace <= 0 {
		cfg.ReuseGrace = 30 * time.Second
	}
	if cfg.VerifyTTL <= 0 {
		cfg.VerifyTTL = 24 * time.Hour
	}
	if cfg.PasswordResetTTL <= 0 {
		cfg.PasswordResetTTL = 1 * time.Hour
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = 720 * time.Hour
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:  store,
		access: access,
		cfg:    cfg,
		log:    log,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// RegisterInput is the registration request payload.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

// RegisterResult is returned after successful registration.
type RegisterResult struct {
	AccountID            uuid.UUID
	Player               PlayerPublic
	VerificationRequired bool
	DevVerificationToken string `json:"dev_verification_token,omitempty"`
	EmailDelivery        string `json:"email_delivery"`
}

// Register creates an account + player and issues an email verification token.
func (s *Service) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return RegisterResult{}, ErrInvalidInput
	}
	if err := validatePassword(in.Password); err != nil {
		return RegisterResult{}, err
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if err := validateDisplayName(displayName); err != nil {
		return RegisterResult{}, err
	}

	passwordHash, err := security.HashPassword(in.Password)
	if err != nil {
		return RegisterResult{}, err
	}
	rawToken, tokenHash, err := security.NewOpaqueToken()
	if err != nil {
		return RegisterResult{}, err
	}

	now := s.now()
	account, player, err := s.store.CreateAccountWithPlayer(
		ctx,
		email,
		passwordHash,
		displayName,
		"default",
		tokenHash,
		now.Add(s.cfg.VerifyTTL),
	)
	if err != nil {
		return RegisterResult{}, err
	}

	result := RegisterResult{
		AccountID:            account.ID,
		Player:               toPlayerPublic(player),
		VerificationRequired: true,
		EmailDelivery:        s.emailDeliveryStatus(),
	}
	if s.cfg.EmailMode == EmailModeDev {
		result.DevVerificationToken = rawToken
	}

	s.log.Info("account_registered",
		"account_id", account.ID.String(),
		"player_id", player.ID.String(),
		"email_delivery", result.EmailDelivery,
	)
	return result, nil
}

func (s *Service) emailDeliveryStatus() string {
	switch s.cfg.EmailMode {
	case EmailModeDev:
		return "dev_token_returned"
	default:
		return "not_configured"
	}
}

// LoginInput is the login request payload.
type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        *netip.Addr
}

// AuthResult is returned by login and refresh.
type AuthResult struct {
	Tokens TokenPair
	Player PlayerPublic
	Me     MeResponse
}

// Login authenticates email/password and issues token pair.
func (s *Service) Login(ctx context.Context, in LoginInput) (AuthResult, error) {
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}
	account, err := s.store.GetAccountByEmail(ctx, email)
	if err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	ok, err := security.VerifyPassword(in.Password, account.PasswordHash)
	if err != nil || !ok {
		s.log.Info("login_failed", "reason", "bad_credentials")
		return AuthResult{}, ErrInvalidCredentials
	}
	if err := s.ensureAccountCanAuthenticate(account); err != nil {
		return AuthResult{}, err
	}

	player, err := s.store.GetPlayerByAccountID(ctx, account.ID)
	if err != nil {
		return AuthResult{}, err
	}

	tokens, err := s.issueSession(ctx, account.ID, player.ID, in.UserAgent, in.IP)
	if err != nil {
		return AuthResult{}, err
	}

	s.log.Info("login_success", "account_id", account.ID.String(), "session_id", tokens.SessionID.String())
	return AuthResult{
		Tokens: tokens,
		Player: toPlayerPublic(player),
		Me:     toMe(account, player),
	}, nil
}

func (s *Service) ensureAccountCanAuthenticate(account Account) error {
	switch account.Status {
	case AccountActive:
		return nil
	case AccountPendingVerification:
		return ErrAccountNotVerified
	case AccountLocked:
		return ErrAccountLocked
	case AccountClosed:
		return ErrAccountClosed
	default:
		return ErrAccountLocked
	}
}

func (s *Service) issueSession(
	ctx context.Context,
	accountID, playerID uuid.UUID,
	userAgent string,
	ip *netip.Addr,
) (TokenPair, error) {
	now := s.now()
	rawRefresh, refreshHash, err := security.NewOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}
	session, err := s.store.CreateSession(ctx, accountID, refreshHash, now.Add(s.cfg.RefreshTTL), userAgent, ip)
	if err != nil {
		return TokenPair{}, err
	}
	access, accessExp, err := s.access.Issue(accountID, playerID, session.ID, now)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      access,
		RefreshToken:     rawRefresh,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: session.ExpiresAt,
		SessionID:        session.ID,
	}, nil
}

// RefreshInput carries a refresh token presentation.
type RefreshInput struct {
	RefreshToken string
	UserAgent    string
	IP           *netip.Addr
}

// Refresh rotates the refresh token and issues a new access token.
func (s *Service) Refresh(ctx context.Context, in RefreshInput) (AuthResult, error) {
	if strings.TrimSpace(in.RefreshToken) == "" {
		return AuthResult{}, ErrInvalidCredentials
	}
	now := s.now()
	hash := security.HashOpaqueToken(in.RefreshToken)

	session, err := s.store.GetSessionByRefreshHash(ctx, hash)
	if err != nil {
		if err := s.handleRefreshMiss(ctx, hash, now); err != nil {
			return AuthResult{}, err
		}
		return AuthResult{}, ErrInvalidCredentials
	}
	if session.RevokedAt != nil {
		_, _ = s.store.RevokeAllSessions(ctx, session.AccountID, "refresh_reuse", now)
		s.log.Info("refresh_reuse_detected", "account_id", session.AccountID.String(), "session_id", session.ID.String())
		return AuthResult{}, ErrInvalidCredentials
	}
	if !session.ExpiresAt.After(now) {
		return AuthResult{}, ErrSessionExpired
	}

	account, err := s.store.GetAccountByID(ctx, session.AccountID)
	if err != nil {
		return AuthResult{}, err
	}
	if err := s.ensureAccountCanAuthenticate(account); err != nil {
		return AuthResult{}, err
	}
	player, err := s.store.GetPlayerByAccountID(ctx, account.ID)
	if err != nil {
		return AuthResult{}, err
	}

	rawRefresh, newHash, err := security.NewOpaqueToken()
	if err != nil {
		return AuthResult{}, err
	}
	rotated, err := s.store.RotateSession(ctx, session.ID, hash, newHash, now.Add(s.cfg.RefreshTTL), now)
	if err != nil {
		return AuthResult{}, err
	}
	access, accessExp, err := s.access.Issue(account.ID, player.ID, rotated.ID, now)
	if err != nil {
		return AuthResult{}, err
	}

	s.log.Info("session_refreshed", "account_id", account.ID.String(), "session_id", rotated.ID.String())
	return AuthResult{
		Tokens: TokenPair{
			AccessToken:      access,
			RefreshToken:     rawRefresh,
			AccessExpiresAt:  accessExp,
			RefreshExpiresAt: rotated.ExpiresAt,
			SessionID:        rotated.ID,
		},
		Player: toPlayerPublic(player),
		Me:     toMe(account, player),
	}, nil
}

func (s *Service) handleRefreshMiss(ctx context.Context, hash []byte, now time.Time) error {
	prev, err := s.store.GetSessionByPreviousRefreshHash(ctx, hash)
	if err != nil {
		return nil
	}
	if prev.RotatedAt != nil && now.Sub(*prev.RotatedAt) <= s.cfg.ReuseGrace {
		// Likely concurrent refresh; do not escalate to full revocation.
		return ErrInvalidCredentials
	}
	_, _ = s.store.RevokeAllSessions(ctx, prev.AccountID, "refresh_reuse", now)
	s.log.Info("refresh_reuse_detected", "account_id", prev.AccountID.String(), "session_id", prev.ID.String())
	return ErrInvalidCredentials
}

// Logout revokes the current session identified by access-token claims.
func (s *Service) Logout(ctx context.Context, accountID, sessionID uuid.UUID) error {
	err := s.store.RevokeSession(ctx, accountID, sessionID, "logout", s.now())
	if err != nil {
		return err
	}
	s.log.Info("logout", "account_id", accountID.String(), "session_id", sessionID.String())
	return nil
}

// LogoutAll revokes every active session for the account.
func (s *Service) LogoutAll(ctx context.Context, accountID uuid.UUID) error {
	_, err := s.store.RevokeAllSessions(ctx, accountID, "logout_all", s.now())
	if err != nil {
		return err
	}
	s.log.Info("logout_all", "account_id", accountID.String())
	return nil
}

// Me loads the authenticated account and player profile.
func (s *Service) Me(ctx context.Context, accountID uuid.UUID) (MeResponse, error) {
	account, err := s.store.GetAccountByID(ctx, accountID)
	if err != nil {
		return MeResponse{}, err
	}
	player, err := s.store.GetPlayerByAccountID(ctx, accountID)
	if err != nil {
		return MeResponse{}, err
	}
	return toMe(account, player), nil
}

// UpdateProfileInput patches profile fields.
type UpdateProfileInput struct {
	DisplayName *string
	AvatarID    *string
	Bio         *string
}

// UpdateProfile updates the authenticated player's profile.
func (s *Service) UpdateProfile(ctx context.Context, accountID uuid.UUID, in UpdateProfileInput) (PlayerPublic, error) {
	if in.DisplayName == nil && in.AvatarID == nil && in.Bio == nil {
		return PlayerPublic{}, ErrInvalidInput
	}
	var displayName, avatarID, bio *string
	if in.DisplayName != nil {
		v := strings.TrimSpace(*in.DisplayName)
		if err := validateDisplayName(v); err != nil {
			return PlayerPublic{}, err
		}
		displayName = &v
	}
	if in.AvatarID != nil {
		v := strings.TrimSpace(*in.AvatarID)
		if err := validateAvatarID(v); err != nil {
			return PlayerPublic{}, err
		}
		avatarID = &v
	}
	if in.Bio != nil {
		v := *in.Bio
		if err := validateBio(v); err != nil {
			return PlayerPublic{}, err
		}
		bio = &v
	}
	player, err := s.store.UpdatePlayerProfile(ctx, accountID, displayName, avatarID, bio, s.now())
	if err != nil {
		return PlayerPublic{}, err
	}
	return toPlayerPublic(player), nil
}

// ListSessions returns active sessions for the account.
func (s *Service) ListSessions(ctx context.Context, accountID, currentSessionID uuid.UUID) ([]SessionView, error) {
	sessions, err := s.store.ListSessions(ctx, accountID, false)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(sessions))
	for _, session := range sessions {
		view := SessionView{
			ID:        session.ID,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
			UserAgent: session.UserAgent,
			Current:   session.ID == currentSessionID,
			RevokedAt: session.RevokedAt,
		}
		if session.IP != nil {
			view.IP = session.IP.String()
		}
		out = append(out, view)
	}
	return out, nil
}

// RevokeSession revokes a specific session owned by the account.
func (s *Service) RevokeSession(ctx context.Context, accountID, sessionID uuid.UUID) error {
	if err := s.store.RevokeSession(ctx, accountID, sessionID, "user_revoke", s.now()); err != nil {
		return err
	}
	s.log.Info("session_revoked", "account_id", accountID.String(), "session_id", sessionID.String())
	return nil
}

// VerifyEmail consumes a verification token and activates the account.
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return ErrTokenInvalid
	}
	_, account, err := s.store.ConsumeEmailActionToken(ctx, security.HashOpaqueToken(rawToken), PurposeVerifyEmail, s.now())
	if err != nil {
		return err
	}
	s.log.Info("email_verified", "account_id", account.ID.String())
	return nil
}

// ForgotPasswordInput starts account recovery without revealing whether the email exists.
type ForgotPasswordInput struct {
	Email string
}

// ForgotPasswordResult is intentionally generic unless EmailModeDev is enabled for a known account.
type ForgotPasswordResult struct {
	EmailDelivery string `json:"email_delivery"`
	DevResetToken string `json:"dev_reset_token,omitempty"`
	Accepted      bool   `json:"accepted"`
}

// ForgotPassword creates a reset token when the account exists. Responses avoid account enumeration.
func (s *Service) ForgotPassword(ctx context.Context, in ForgotPasswordInput) (ForgotPasswordResult, error) {
	result := ForgotPasswordResult{
		Accepted:      true,
		EmailDelivery: s.emailDeliveryStatus(),
	}
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		// Still return generic success to avoid enumeration via validation differences on format-only misses.
		return result, nil
	}
	account, err := s.store.GetAccountByEmail(ctx, email)
	if err != nil {
		return result, nil
	}
	if account.Status == AccountClosed || account.Status == AccountLocked {
		return result, nil
	}

	rawToken, tokenHash, err := security.NewOpaqueToken()
	if err != nil {
		return ForgotPasswordResult{}, err
	}
	now := s.now()
	if err := s.store.CreatePasswordResetToken(ctx, account.ID, tokenHash, now.Add(s.cfg.PasswordResetTTL), now); err != nil {
		return ForgotPasswordResult{}, err
	}
	if s.cfg.EmailMode == EmailModeDev {
		result.DevResetToken = rawToken
	}
	s.log.Info("password_reset_requested", "account_id", account.ID.String(), "email_delivery", result.EmailDelivery)
	return result, nil
}

// ResetPasswordInput completes password recovery.
type ResetPasswordInput struct {
	Token       string
	NewPassword string
}

// ResetPassword consumes a reset token, updates the password, and revokes sessions.
func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput) error {
	if err := validatePassword(in.NewPassword); err != nil {
		return err
	}
	if strings.TrimSpace(in.Token) == "" {
		return ErrTokenInvalid
	}
	token, account, err := s.store.ConsumeEmailActionToken(ctx, security.HashOpaqueToken(in.Token), PurposePasswordReset, s.now())
	if err != nil {
		return err
	}
	_ = token
	hash, err := security.HashPassword(in.NewPassword)
	if err != nil {
		return err
	}
	if err := s.store.UpdatePasswordHash(ctx, account.ID, hash, s.now()); err != nil {
		return err
	}
	s.log.Info("password_reset_completed", "account_id", account.ID.String())
	return nil
}

// ParseAccessToken validates a bearer access token.
func (s *Service) ParseAccessToken(token string) (*AccessClaims, error) {
	return s.access.Parse(token)
}

// EnsureSessionActive confirms the session referenced by an access token is still valid.
func (s *Service) EnsureSessionActive(ctx context.Context, sessionID uuid.UUID) error {
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return ErrUnauthorized
	}
	now := s.now()
	if session.RevokedAt != nil {
		return ErrUnauthorized
	}
	if !session.ExpiresAt.After(now) {
		return ErrSessionExpired
	}
	return nil
}

func toPlayerPublic(p Player) PlayerPublic {
	return PlayerPublic{
		PlayerID:    p.ID,
		DisplayName: p.DisplayName,
		AvatarID:    p.AvatarID,
		Bio:         p.Bio,
		CreatedAt:   p.CreatedAt.UTC(),
		UpdatedAt:   p.UpdatedAt.UTC(),
	}
}

func toMe(account Account, player Player) MeResponse {
	return MeResponse{
		AccountID:     account.ID,
		Email:         account.EmailNormalized,
		Status:        account.Status,
		EmailVerified: account.EmailVerifiedAt != nil,
		Player:        toPlayerPublic(player),
	}
}
