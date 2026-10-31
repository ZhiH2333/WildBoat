package room

import "errors"

// Domain errors mapped to stable API codes by handlers.
var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrNotFound              = errors.New("room not found")
	ErrCodeInvalid           = errors.New("room code invalid")
	ErrFull                  = errors.New("room full")
	ErrNotJoinable           = errors.New("room not joinable")
	ErrAlreadyIn             = errors.New("already in a room")
	ErrNotReady              = errors.New("room not ready")
	ErrAlreadyStarted        = errors.New("room already started")
	ErrNotOwner              = errors.New("not room owner")
	ErrAssignmentNotReady    = errors.New("assignment not ready")
	ErrMatchAlreadyActive    = errors.New("match already active")
	ErrGameServerUnavailable = errors.New("game server unavailable")
	ErrGameCapacityFull      = errors.New("game capacity full")
)
