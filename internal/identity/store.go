package identity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists WPass identity entities in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a PostgreSQL-backed identity store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateAccountWithPlayer inserts an account, player, and email verification token atomically.
func (s *Store) CreateAccountWithPlayer(
	ctx context.Context,
	emailNormalized, passwordHash, displayName, avatarID string,
	verifyTokenHash []byte,
	verifyExpiresAt time.Time,
) (Account, Player, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, Player{}, fmt.Errorf("begin register tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var account Account
	err = tx.QueryRow(ctx, `
		INSERT INTO accounts (email_normalized, password_hash, status)
		VALUES ($1, $2, $3)
		RETURNING id, email_normalized, password_hash, status, email_verified_at, created_at, updated_at
	`, emailNormalized, passwordHash, AccountPendingVerification).Scan(
		&account.ID,
		&account.EmailNormalized,
		&account.PasswordHash,
		&account.Status,
		&account.EmailVerifiedAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Account{}, Player{}, ErrEmailTaken
		}
		return Account{}, Player{}, fmt.Errorf("insert account: %w", err)
	}

	var player Player
	err = tx.QueryRow(ctx, `
		INSERT INTO players (account_id, display_name, avatar_id)
		VALUES ($1, $2, $3)
		RETURNING id, account_id, display_name, avatar_id, bio, profile_version, created_at, updated_at
	`, account.ID, displayName, avatarID).Scan(
		&player.ID,
		&player.AccountID,
		&player.DisplayName,
		&player.AvatarID,
		&player.Bio,
		&player.ProfileVersion,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if err != nil {
		return Account{}, Player{}, fmt.Errorf("insert player: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO email_action_tokens (account_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, $3, $4)
	`, account.ID, verifyTokenHash, PurposeVerifyEmail, verifyExpiresAt)
	if err != nil {
		return Account{}, Player{}, fmt.Errorf("insert verify token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Account{}, Player{}, fmt.Errorf("commit register tx: %w", err)
	}
	return account, player, nil
}

// GetAccountByEmail loads an account by normalized email.
func (s *Store) GetAccountByEmail(ctx context.Context, emailNormalized string) (Account, error) {
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, email_normalized, password_hash, status, email_verified_at, created_at, updated_at
		FROM accounts WHERE email_normalized = $1
	`, emailNormalized).Scan(
		&account.ID,
		&account.EmailNormalized,
		&account.PasswordHash,
		&account.Status,
		&account.EmailVerifiedAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account by email: %w", err)
	}
	return account, nil
}

// GetAccountByID loads an account by ID.
func (s *Store) GetAccountByID(ctx context.Context, id uuid.UUID) (Account, error) {
	var account Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, email_normalized, password_hash, status, email_verified_at, created_at, updated_at
		FROM accounts WHERE id = $1
	`, id).Scan(
		&account.ID,
		&account.EmailNormalized,
		&account.PasswordHash,
		&account.Status,
		&account.EmailVerifiedAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account by id: %w", err)
	}
	return account, nil
}

// GetPlayerByAccountID loads the player profile for an account.
func (s *Store) GetPlayerByAccountID(ctx context.Context, accountID uuid.UUID) (Player, error) {
	var player Player
	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, display_name, avatar_id, bio, profile_version, created_at, updated_at
		FROM players WHERE account_id = $1
	`, accountID).Scan(
		&player.ID,
		&player.AccountID,
		&player.DisplayName,
		&player.AvatarID,
		&player.Bio,
		&player.ProfileVersion,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Player{}, ErrNotFound
	}
	if err != nil {
		return Player{}, fmt.Errorf("get player: %w", err)
	}
	return player, nil
}

// CreateSession inserts a new refresh session.
func (s *Store) CreateSession(
	ctx context.Context,
	accountID uuid.UUID,
	refreshHash []byte,
	expiresAt time.Time,
	userAgent string,
	ip *netip.Addr,
) (Session, error) {
	var session Session
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (account_id, refresh_token_hash, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, account_id, refresh_token_hash, previous_refresh_token_hash, rotated_at,
		          user_agent, ip, created_at, expires_at, revoked_at, revoke_reason
	`, accountID, refreshHash, userAgent, ip, expiresAt).Scan(
		&session.ID,
		&session.AccountID,
		&session.RefreshTokenHash,
		&session.PreviousRefreshTokenHash,
		&session.RotatedAt,
		&session.UserAgent,
		&session.IP,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.RevokeReason,
	)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

// GetSessionByID loads a session by ID.
func (s *Store) GetSessionByID(ctx context.Context, id uuid.UUID) (Session, error) {
	return s.scanSession(s.pool.QueryRow(ctx, sessionSelect+` WHERE id = $1`, id))
}

// GetSessionByRefreshHash loads a session by current refresh-token hash.
func (s *Store) GetSessionByRefreshHash(ctx context.Context, hash []byte) (Session, error) {
	return s.scanSession(s.pool.QueryRow(ctx, sessionSelect+` WHERE refresh_token_hash = $1`, hash))
}

// GetSessionByPreviousRefreshHash loads a session by previous refresh-token hash (reuse detection).
func (s *Store) GetSessionByPreviousRefreshHash(ctx context.Context, hash []byte) (Session, error) {
	return s.scanSession(s.pool.QueryRow(ctx, sessionSelect+` WHERE previous_refresh_token_hash = $1`, hash))
}

const sessionSelect = `
	SELECT id, account_id, refresh_token_hash, previous_refresh_token_hash, rotated_at,
	       user_agent, ip, created_at, expires_at, revoked_at, revoke_reason
	FROM sessions
`

func (s *Store) scanSession(row pgx.Row) (Session, error) {
	var session Session
	err := row.Scan(
		&session.ID,
		&session.AccountID,
		&session.RefreshTokenHash,
		&session.PreviousRefreshTokenHash,
		&session.RotatedAt,
		&session.UserAgent,
		&session.IP,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.RevokeReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("scan session: %w", err)
	}
	return session, nil
}

// RotateSession updates the refresh-token hash in place under a row lock.
func (s *Store) RotateSession(
	ctx context.Context,
	sessionID uuid.UUID,
	currentHash []byte,
	newHash []byte,
	newExpiresAt time.Time,
	now time.Time,
) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin rotate tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var session Session
	err = tx.QueryRow(ctx, sessionSelect+` WHERE id = $1 FOR UPDATE`, sessionID).Scan(
		&session.ID,
		&session.AccountID,
		&session.RefreshTokenHash,
		&session.PreviousRefreshTokenHash,
		&session.RotatedAt,
		&session.UserAgent,
		&session.IP,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.RevokeReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("lock session: %w", err)
	}
	if session.RevokedAt != nil {
		return Session{}, ErrSessionRevoked
	}
	if !session.ExpiresAt.After(now) {
		return Session{}, ErrSessionExpired
	}
	if !bytes.Equal(session.RefreshTokenHash, currentHash) {
		return Session{}, ErrInvalidCredentials
	}

	err = tx.QueryRow(ctx, `
		UPDATE sessions
		SET previous_refresh_token_hash = refresh_token_hash,
		    refresh_token_hash = $2,
		    rotated_at = $3,
		    expires_at = $4
		WHERE id = $1
		RETURNING id, account_id, refresh_token_hash, previous_refresh_token_hash, rotated_at,
		          user_agent, ip, created_at, expires_at, revoked_at, revoke_reason
	`, sessionID, newHash, now, newExpiresAt).Scan(
		&session.ID,
		&session.AccountID,
		&session.RefreshTokenHash,
		&session.PreviousRefreshTokenHash,
		&session.RotatedAt,
		&session.UserAgent,
		&session.IP,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.RevokeReason,
	)
	if err != nil {
		return Session{}, fmt.Errorf("rotate session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit rotate tx: %w", err)
	}
	return session, nil
}

// RevokeSession marks a session revoked if it belongs to the account.
func (s *Store) RevokeSession(ctx context.Context, accountID, sessionID uuid.UUID, reason string, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $3, revoke_reason = $4
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
	`, sessionID, accountID, now, reason)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAllSessions revokes every active session for an account.
func (s *Store) RevokeAllSessions(ctx context.Context, accountID uuid.UUID, reason string, now time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $2, revoke_reason = $3
		WHERE account_id = $1 AND revoked_at IS NULL
	`, accountID, now, reason)
	if err != nil {
		return 0, fmt.Errorf("revoke all sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListSessions returns sessions for an account, newest first.
func (s *Store) ListSessions(ctx context.Context, accountID uuid.UUID, includeRevoked bool) ([]Session, error) {
	query := sessionSelect + ` WHERE account_id = $1`
	if !includeRevoked {
		query += ` AND revoked_at IS NULL`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		session, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

// UpdatePlayerProfile patches mutable profile fields.
func (s *Store) UpdatePlayerProfile(
	ctx context.Context,
	accountID uuid.UUID,
	displayName, avatarID, bio *string,
	now time.Time,
) (Player, error) {
	player, err := s.GetPlayerByAccountID(ctx, accountID)
	if err != nil {
		return Player{}, err
	}
	if displayName != nil {
		player.DisplayName = *displayName
	}
	if avatarID != nil {
		player.AvatarID = *avatarID
	}
	if bio != nil {
		player.Bio = *bio
	}

	err = s.pool.QueryRow(ctx, `
		UPDATE players
		SET display_name = $2,
		    avatar_id = $3,
		    bio = $4,
		    profile_version = profile_version + 1,
		    updated_at = $5
		WHERE account_id = $1
		RETURNING id, account_id, display_name, avatar_id, bio, profile_version, created_at, updated_at
	`, accountID, player.DisplayName, player.AvatarID, player.Bio, now).Scan(
		&player.ID,
		&player.AccountID,
		&player.DisplayName,
		&player.AvatarID,
		&player.Bio,
		&player.ProfileVersion,
		&player.CreatedAt,
		&player.UpdatedAt,
	)
	if err != nil {
		return Player{}, fmt.Errorf("update player: %w", err)
	}
	return player, nil
}

// ConsumeEmailActionToken validates and consumes a single-use email action token.
func (s *Store) ConsumeEmailActionToken(
	ctx context.Context,
	tokenHash []byte,
	purpose string,
	now time.Time,
) (EmailActionToken, Account, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return EmailActionToken{}, Account{}, fmt.Errorf("begin consume token tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var token EmailActionToken
	err = tx.QueryRow(ctx, `
		SELECT id, account_id, token_hash, purpose, expires_at, consumed_at, created_at
		FROM email_action_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, tokenHash).Scan(
		&token.ID,
		&token.AccountID,
		&token.TokenHash,
		&token.Purpose,
		&token.ExpiresAt,
		&token.ConsumedAt,
		&token.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return EmailActionToken{}, Account{}, ErrTokenInvalid
	}
	if err != nil {
		return EmailActionToken{}, Account{}, fmt.Errorf("load email action token: %w", err)
	}
	if token.Purpose != purpose {
		return EmailActionToken{}, Account{}, ErrTokenInvalid
	}
	if token.ConsumedAt != nil {
		return EmailActionToken{}, Account{}, ErrTokenConsumed
	}
	if !token.ExpiresAt.After(now) {
		return EmailActionToken{}, Account{}, ErrTokenExpired
	}

	_, err = tx.Exec(ctx, `
		UPDATE email_action_tokens SET consumed_at = $2 WHERE id = $1
	`, token.ID, now)
	if err != nil {
		return EmailActionToken{}, Account{}, fmt.Errorf("consume token: %w", err)
	}

	var account Account
	err = tx.QueryRow(ctx, `
		SELECT id, email_normalized, password_hash, status, email_verified_at, created_at, updated_at
		FROM accounts WHERE id = $1 FOR UPDATE
	`, token.AccountID).Scan(
		&account.ID,
		&account.EmailNormalized,
		&account.PasswordHash,
		&account.Status,
		&account.EmailVerifiedAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return EmailActionToken{}, Account{}, fmt.Errorf("load account for token: %w", err)
	}

	if purpose == PurposeVerifyEmail {
		_, err = tx.Exec(ctx, `
			UPDATE accounts
			SET status = CASE WHEN status = $2 THEN $3 ELSE status END,
			    email_verified_at = COALESCE(email_verified_at, $4),
			    updated_at = $4
			WHERE id = $1
		`, account.ID, AccountPendingVerification, AccountActive, now)
		if err != nil {
			return EmailActionToken{}, Account{}, fmt.Errorf("mark verified: %w", err)
		}
		account, err = scanAccount(tx.QueryRow(ctx, `
			SELECT id, email_normalized, password_hash, status, email_verified_at, created_at, updated_at
			FROM accounts WHERE id = $1
		`, account.ID))
		if err != nil {
			return EmailActionToken{}, Account{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return EmailActionToken{}, Account{}, fmt.Errorf("commit consume token: %w", err)
	}
	return token, account, nil
}

func scanAccount(row pgx.Row) (Account, error) {
	var account Account
	err := row.Scan(
		&account.ID,
		&account.EmailNormalized,
		&account.PasswordHash,
		&account.Status,
		&account.EmailVerifiedAt,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if err != nil {
		return Account{}, fmt.Errorf("scan account: %w", err)
	}
	return account, nil
}

// CreatePasswordResetToken inserts a password-reset action token and invalidates prior unused ones.
func (s *Store) CreatePasswordResetToken(
	ctx context.Context,
	accountID uuid.UUID,
	tokenHash []byte,
	expiresAt time.Time,
	now time.Time,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reset token tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE email_action_tokens
		SET consumed_at = $3
		WHERE account_id = $1 AND purpose = $2 AND consumed_at IS NULL
	`, accountID, PurposePasswordReset, now)
	if err != nil {
		return fmt.Errorf("invalidate prior reset tokens: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO email_action_tokens (account_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, $3, $4)
	`, accountID, tokenHash, PurposePasswordReset, expiresAt)
	if err != nil {
		return fmt.Errorf("insert reset token: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdatePasswordHash sets a new password hash and revokes all sessions.
func (s *Store) UpdatePasswordHash(ctx context.Context, accountID uuid.UUID, passwordHash string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE accounts SET password_hash = $2, updated_at = $3 WHERE id = $1
	`, accountID, passwordHash, now)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	_, err = tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = $2, revoke_reason = $3
		WHERE account_id = $1 AND revoked_at IS NULL
	`, accountID, now, "password_reset")
	if err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	return tx.Commit(ctx)
}
