package user

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"

	"admission-api/internal/platform/middleware"
	"admission-api/internal/platform/web"
)

// Request/Response DTOs for Swagger.

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email" example:"user@example.com"`
	Password string `json:"password" validate:"required,min=8,alphanum" example:"pass1234"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email" example:"user@example.com"`
	Password string `json:"password" validate:"required,min=8,alphanum" example:"pass1234"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

type Response struct {
	ID        int64     `json:"id" example:"1"`
	Email     string    `json:"email" example:"user@example.com"`
	Role      string    `json:"role" example:"user"`
	CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	RefreshToken string `json:"refresh_token" example:"abc123..."`
	ExpiresIn    int    `json:"expires_in" example:"900"`
}

type Handler struct {
	web.BaseHandler
	service   Service
	jwtConfig *middleware.JWTConfig
	validate  *validator.Validate
}

func NewHandler(service Service, jwtConfig *middleware.JWTConfig) *Handler {
	return &Handler{
		service:   service,
		jwtConfig: jwtConfig,
		validate:  validator.New(),
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
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	u, err := h.service.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		if strings.Contains(err.Error(), "email already exists") {
			h.RespondError(w, http.StatusConflict, web.ErrCodeConflict, "email already exists")
			return
		}
		h.RespondError(w, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(w, http.StatusOK, web.SuccessResponse(Response{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
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
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	platform := middleware.PlatformFromContext(r.Context())

	tokens, err := h.service.Login(r.Context(), req.Email, req.Password, platform)
	if err != nil {
		h.RespondError(w, http.StatusUnauthorized, web.ErrCodeUnauthorized, "invalid credentials")
		return
	}

	h.RespondJSON(w, http.StatusOK, web.SuccessResponse(TokenResponse{
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
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, "invalid request body")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.RespondError(w, http.StatusBadRequest, web.ErrCodeBadRequest, err.Error())
		return
	}

	tokens, err := h.service.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.RespondError(w, http.StatusUnauthorized, web.ErrCodeUnauthorized, "invalid or expired refresh token")
		return
	}

	h.RespondJSON(w, http.StatusOK, web.SuccessResponse(TokenResponse{
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
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := middleware.UserFromContext(r.Context())
	if !ok {
		h.RespondError(w, http.StatusUnauthorized, web.ErrCodeUnauthorized, "unauthorized")
		return
	}

	u, err := h.service.Me(r.Context(), userID)
	if err != nil {
		h.RespondError(w, http.StatusInternalServerError, web.ErrCodeInternal, "internal server error")
		return
	}

	h.RespondJSON(w, http.StatusOK, web.SuccessResponse(Response{
		ID:        u.ID,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
	}))
}
