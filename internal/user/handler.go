package user

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

// Request/Response DTOs for Swagger.

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email" example:"user@example.com"`
	Password string `json:"password" validate:"required,min=8,alphanum" example:"pass1234"`
	UserType string `json:"user_type" validate:"required,oneof=parent student" example:"parent"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email" example:"user@example.com"`
	Password string `json:"password" validate:"required,min=8,alphanum" example:"pass1234"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=8,alphanum" example:"oldpass123"`
	NewPassword     string `json:"new_password" validate:"required,min=8,alphanum" example:"newpass123"`
}

type SendPhoneCodeRequest struct {
	Phone string `json:"phone" validate:"required" example:"13800138000"`
}

type VerifyPhoneRequest struct {
	Phone string `json:"phone" validate:"required" example:"13800138000"`
	Code  string `json:"code" validate:"required,len=6,numeric" example:"123456"`
}

type Response struct {
	ID            int64     `json:"id" example:"1"`
	Email         string    `json:"email" example:"user@example.com"`
	Username      string    `json:"username" example:"johndoe"`
	Phone         string    `json:"phone" example:"13800138000"`
	PhoneVerified bool      `json:"phone_verified" example:"true"`
	Role          string    `json:"role" example:"user"`
	UserType      string    `json:"user_type" example:"parent"`
	Status        string    `json:"status" example:"active"`
	CreatedAt     time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"abc123..."`
	ExpiresIn    int    `json:"expires_in" example:"900"`
}

type Handler struct {
	web.BaseHandler
	service                  Service
	phoneVerificationService PhoneVerificationService
	jwtConfig                *middleware.JWTConfig
	validate                 *validator.Validate
}

func NewHandler(service Service, phoneVerificationService PhoneVerificationService, jwtConfig *middleware.JWTConfig) *Handler {
	return &Handler{
		service:                  service,
		phoneVerificationService: phoneVerificationService,
		jwtConfig:                jwtConfig,
		validate:                 validator.New(),
	}
}

// Register godoc
// @Summary      用户注册
// @Description  使用邮箱和密码注册新账户
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RegisterRequest  true  "注册信息"
// @Success      200   {object}  web.Response{data=Response}
// @Failure      400   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/auth/register [post]
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	u, err := h.service.Register(c.Request.Context(), req.Email, req.Password, req.UserType)
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyExists) {
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "email already exists")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(Response{
		ID:            u.ID,
		Email:         u.Email,
		Username:      stringValue(u.Username),
		Phone:         stringValue(u.Phone),
		PhoneVerified: u.PhoneVerifiedAt != nil,
		Role:          u.Role,
		UserType:      u.UserType,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt,
	}))
}

// Login godoc
// @Summary      用户登录
// @Description  使用邮箱和密码登录，获取 Access Token 和 Refresh Token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest  true  "登录信息"
// @Success      200   {object}  web.Response{data=TokenResponse}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Router       /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	platform := "web"
	if p, ok := c.Get(middleware.ContextPlatformKey); ok {
		if ps, ok := p.(string); ok {
			platform = ps
		}
	}

	tokens, err := h.service.Login(c.Request.Context(), req.Email, req.Password, platform)
	if err != nil {
		slog.Warn("login failed", "email", req.Email, "error", err.Error())
		switch {
		case errors.Is(err, ErrAccountBanned):
			h.RespondError(c, http.StatusForbidden, web.ErrCodeForbidden, "account has been banned")
		case errors.Is(err, ErrInvalidCredentials):
			h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "invalid credentials")
		default:
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		}
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(TokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    tokens.ExpiresIn,
	}))
}

// Refresh godoc
// @Summary      刷新 Access Token
// @Description  用 Refresh Token 换取新的双 Token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      RefreshRequest  true  "Refresh Token"
// @Success      200   {object}  web.Response{data=TokenResponse}
// @Failure      401   {object}  web.Response
// @Router       /api/v1/auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	tokens, err := h.service.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "invalid or expired refresh token")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(TokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    tokens.ExpiresIn,
	}))
}

// Me godoc
// @Summary      获取当前用户信息
// @Description  返回当前登录用户的个人信息
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  web.Response{data=Response}
// @Failure      401  {object}  web.Response
// @Router       /api/v1/me [get]
func (h *Handler) Me(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	u, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(Response{
		ID:            u.ID,
		Email:         u.Email,
		Username:      stringValue(u.Username),
		Phone:         stringValue(u.Phone),
		PhoneVerified: u.PhoneVerifiedAt != nil,
		Role:          u.Role,
		UserType:      u.UserType,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt,
	}))
}

// ChangePassword godoc
// @Summary      用户修改自己的密码
// @Description  当前登录用户通过旧密码校验后修改自己的密码
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      ChangePasswordRequest  true  "密码信息"
// @Success      200   {object}  web.Response{data=map[string]string}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Router       /api/v1/me/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	if req.CurrentPassword == req.NewPassword {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "new password must be different from current password")
		return
	}

	if err := h.service.ChangePassword(c.Request.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "current password is incorrect")
			return
		}
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "password changed"}))
}

// SendPhoneVerificationCode godoc
// @Summary      发送手机号验证码
// @Description  当前登录用户向指定手机号发送验证码，用于绑定手机号
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      SendPhoneCodeRequest  true  "手机号"
// @Success      200   {object}  web.Response{data=map[string]string}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/me/phone/send-code [post]
func (h *Handler) SendPhoneVerificationCode(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req SendPhoneCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	if err := h.phoneVerificationService.SendPhoneVerificationCode(c.Request.Context(), userID, req.Phone); err != nil {
		switch {
		case errors.Is(err, ErrPhoneInvalid),
			errors.Is(err, ErrPhoneCodeTooFrequent),
			errors.Is(err, ErrPhoneCodeDailyLimit):
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		case errors.Is(err, ErrPhoneAlreadyExists):
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "phone already exists")
		default:
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		}
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "verification code sent"}))
}

// VerifyPhone godoc
// @Summary      校验手机号验证码
// @Description  当前登录用户校验验证码并完成手机号绑定
// @Tags         user
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      VerifyPhoneRequest  true  "手机号与验证码"
// @Success      200   {object}  web.Response{data=map[string]string}
// @Failure      400   {object}  web.Response
// @Failure      401   {object}  web.Response
// @Failure      409   {object}  web.Response
// @Router       /api/v1/me/phone/verify [post]
func (h *Handler) VerifyPhone(c *gin.Context) {
	userIDRaw, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	userID, ok := userIDRaw.(int64)
	if !ok {
		h.RespondError(c, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	var req VerifyPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}
	if err := h.validate.Struct(req); err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	if err := h.phoneVerificationService.VerifyPhoneCode(c.Request.Context(), userID, req.Phone, req.Code); err != nil {
		switch {
		case errors.Is(err, ErrPhoneInvalid),
			errors.Is(err, ErrVerificationCodeInvalid),
			errors.Is(err, ErrVerificationCodeExpired),
			errors.Is(err, ErrVerificationCodeExceeded):
			h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		case errors.Is(err, ErrPhoneAlreadyExists):
			h.RespondError(c, http.StatusConflict, web.ErrCodeConflict, "phone already exists")
		default:
			h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		}
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(gin.H{"message": "phone verified"}))
}
