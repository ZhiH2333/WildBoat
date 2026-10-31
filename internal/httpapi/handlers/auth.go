package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/identity"
)

// Auth handles WPass authentication routes.
type Auth struct {
	Service *identity.Service
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type tokenResponse struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	SessionID        string    `json:"session_id"`
}

// Register handles POST /v1/auth/register.
func (h Auth) Register(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	result, err := h.Service.Register(r.Context(), identity.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	payload := map[string]any{
		"account_id":            result.AccountID,
		"player":                result.Player,
		"verification_required": result.VerificationRequired,
		"email_delivery":        result.EmailDelivery,
	}
	if result.DevVerificationToken != "" {
		payload["dev_verification_token"] = result.DevVerificationToken
	}
	response.JSON(w, http.StatusCreated, payload)
}

// Login handles POST /v1/auth/login.
func (h Auth) Login(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	result, err := h.Service.Login(r.Context(), identity.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IP:        requestIP(r),
	})
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"tokens": toTokenResponse(result.Tokens),
		"me":     result.Me,
	})
}

// Refresh handles POST /v1/auth/refresh.
func (h Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	result, err := h.Service.Refresh(r.Context(), identity.RefreshInput{
		RefreshToken: req.RefreshToken,
		UserAgent:    r.UserAgent(),
		IP:           requestIP(r),
	})
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"tokens": toTokenResponse(result.Tokens),
		"me":     result.Me,
	})
}

// Logout handles POST /v1/auth/logout.
func (h Auth) Logout(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	sessionID, ok := middleware.SessionIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	if err := h.Service.Logout(r.Context(), accountID, sessionID); err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// LogoutAll handles POST /v1/auth/logout-all.
func (h Auth) LogoutAll(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	if err := h.Service.LogoutAll(r.Context(), accountID); err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// VerifyEmail handles POST /v1/auth/email/verify.
func (h Auth) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req verifyEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	if err := h.Service.VerifyEmail(r.Context(), req.Token); err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ForgotPassword handles POST /v1/auth/password/forgot.
func (h Auth) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req forgotPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	result, err := h.Service.ForgotPassword(r.Context(), identity.ForgotPasswordInput{Email: req.Email})
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	payload := map[string]any{
		"accepted":       result.Accepted,
		"email_delivery": result.EmailDelivery,
	}
	if result.DevResetToken != "" {
		payload["dev_reset_token"] = result.DevResetToken
	}
	response.JSON(w, http.StatusOK, payload)
}

// ResetPassword handles POST /v1/auth/password/reset.
func (h Auth) ResetPassword(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	var req resetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	if err := h.Service.ResetPassword(r.Context(), identity.ResetPasswordInput{
		Token:       req.Token,
		NewPassword: req.NewPassword,
	}); err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func toTokenResponse(tokens identity.TokenPair) tokenResponse {
	return tokenResponse{
		AccessToken:      tokens.AccessToken,
		RefreshToken:     tokens.RefreshToken,
		TokenType:        "Bearer",
		AccessExpiresAt:  tokens.AccessExpiresAt.UTC(),
		RefreshExpiresAt: tokens.RefreshExpiresAt.UTC(),
		SessionID:        tokens.SessionID.String(),
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Reject trailing junk.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}

func requestIP(r *http.Request) *netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}

func writeIdentityError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "request validation failed", requestID, nil)
	case errors.Is(err, identity.ErrEmailTaken):
		// Generic message to reduce account enumeration on registration timing/content.
		response.Error(w, http.StatusConflict, "EMAIL_TAKEN", "unable to register with the provided email", requestID, nil)
	case errors.Is(err, identity.ErrInvalidCredentials):
		response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "invalid email or password", requestID, nil)
	case errors.Is(err, identity.ErrAccountNotVerified):
		response.Error(w, http.StatusForbidden, "ACCOUNT_NOT_VERIFIED", "email verification required", requestID, nil)
	case errors.Is(err, identity.ErrAccountLocked):
		response.Error(w, http.StatusForbidden, "ACCOUNT_LOCKED", "account is locked", requestID, nil)
	case errors.Is(err, identity.ErrAccountClosed):
		response.Error(w, http.StatusForbidden, "ACCOUNT_CLOSED", "account is closed", requestID, nil)
	case errors.Is(err, identity.ErrSessionExpired), errors.Is(err, identity.ErrSessionRevoked):
		response.Error(w, http.StatusUnauthorized, "AUTH_SESSION_EXPIRED", "session is no longer valid", requestID, nil)
	case errors.Is(err, identity.ErrUnauthorized):
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
	case errors.Is(err, identity.ErrForbidden):
		response.Error(w, http.StatusForbidden, "FORBIDDEN", "forbidden", requestID, nil)
	case errors.Is(err, identity.ErrNotFound):
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found", requestID, nil)
	case errors.Is(err, identity.ErrTokenInvalid), errors.Is(err, identity.ErrTokenConsumed):
		response.Error(w, http.StatusBadRequest, "TOKEN_INVALID", "token is invalid", requestID, nil)
	case errors.Is(err, identity.ErrTokenExpired):
		response.Error(w, http.StatusBadRequest, "TOKEN_EXPIRED", "token has expired", requestID, nil)
	default:
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", requestID, nil)
	}
}
