package identity

import "errors"

// Domain / application errors mapped to stable API codes by handlers.
var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrEmailTaken            = errors.New("email taken")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrAccountNotVerified    = errors.New("account not verified")
	ErrAccountLocked         = errors.New("account locked")
	ErrAccountClosed         = errors.New("account closed")
	ErrSessionExpired        = errors.New("session expired")
	ErrSessionRevoked        = errors.New("session revoked")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrNotFound              = errors.New("not found")
	ErrTokenInvalid          = errors.New("token invalid")
	ErrTokenExpired          = errors.New("token expired")
	ErrTokenConsumed         = errors.New("token consumed")
	ErrEmailDeliveryDisabled = errors.New("email delivery disabled")
)
