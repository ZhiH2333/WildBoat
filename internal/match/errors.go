package match

import "errors"

// Domain / application errors mapped to stable API codes by handlers.
var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrNotFound              = errors.New("not found")
	ErrServerAuthInvalid     = errors.New("server auth invalid")
	ErrGameServerUnavailable = errors.New("game server unavailable")
	ErrGameCapacityFull      = errors.New("game capacity full")
	ErrTicketInvalid         = errors.New("ticket invalid")
	ErrTicketExpired         = errors.New("ticket expired")
	ErrTicketReplayed        = errors.New("ticket replayed")
	ErrTicketRevoked         = errors.New("ticket revoked")
	ErrMatchNotSettleable    = errors.New("match not settleable")
)
