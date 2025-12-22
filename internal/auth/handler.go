package auth

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"instant/internal/session"

	"github.com/gin-gonic/gin"
)

// Handler handles authentication-related HTTP requests
type Handler struct {
	service    Service
	sessionMgr session.Manager
}

// NewHandler creates a new authentication handler
func NewHandler(service Service, sessionMgr session.Manager) *Handler {
	return &Handler{
		service:    service,
		sessionMgr: sessionMgr,
	}
}

// RequestCode handles POST /request-code
// @Summary Request verification code
// @Description Generates and sends a verification code to the provided email
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RequestCodeRequest true "Email address"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/request-code [post]
func (h *Handler) RequestCode(c *gin.Context) {
	var req RequestCodeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.service.RequestCode(c.Request.Context(), req.Email)
	if err != nil {
		log.Printf("Failed to request code for %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send verification code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "verification code sent to your email",
	})
}

// VerifyCode handles POST /verify-code
// @Summary Verify code and authenticate
// @Description Verifies the provided code and creates a session
// @Tags auth
// @Accept json
// @Produce json
// @Param request body VerifyCodeRequest true "Email and verification code"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/verify-code [post]
func (h *Handler) VerifyCode(c *gin.Context) {
	var req VerifyCodeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Additional validation for username
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	// Verify the code and get user
	user, err := h.service.VerifyCode(c.Request.Context(), req.Email, req.Code, req.Username)
	if err != nil {
		log.Printf("Failed to verify code for %s: %v", req.Email, err)
		
		// Handle specific errors
		switch err {
		case ErrInvalidCode:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid verification code"})
		case ErrUsernameExists:
			c.JSON(http.StatusConflict, gin.H{
				"error":   "username_taken",
				"message": "This username is already in use",
				"field":   "username",
			})
		case ErrEmailExists:
			c.JSON(http.StatusConflict, gin.H{
				"error":   "email_taken",
				"message": "This email is already registered",
				"field":   "email",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify code"})
		}
		return
	}

	// Get session max age from environment or use default
	const defaultSessionMaxAge = 3600 // 1 hour
	maxAge := defaultSessionMaxAge
	if maxAgeStr := os.Getenv("SESSION_MAX_AGE"); maxAgeStr != "" {
		if parsed, err := strconv.Atoi(maxAgeStr); err == nil {
			maxAge = parsed
		}
	}

	// Create session
	sessionID, err := h.sessionMgr.Create(c.Request.Context(), user.ID, user.Email, maxAge)
	if err != nil {
		log.Printf("Failed to create session for user %s: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	// Set session cookie
	secure := os.Getenv("APP_ENV") == "production"
	c.SetCookie(
		"session_id",
		sessionID,
		maxAge,
		"/",
		"",
		secure,
		true, // httpOnly
	)

	c.JSON(http.StatusOK, AuthResponse{
		User:      user,
		SessionID: sessionID,
	})
}

// Logout handles POST /logout
// @Summary Logout user
// @Description Invalidates the current session
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	// Get session ID from cookie
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "already logged out"})
		return
	}

	// Delete session
	if err := h.sessionMgr.Delete(c.Request.Context(), sessionID); err != nil {
		log.Printf("Failed to delete session %s: %v", sessionID, err)
	}

	// Clear cookie
	c.SetCookie("session_id", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// Health handles GET /health
// @Summary Health check
// @Description Auth service health check
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "auth-service",
	})
}

// UpdateUser handles PATCH /users/:id
// @Summary Update user information
// @Description Updates user email and/or username
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body UpdateUserRequest true "Update fields"
// @Success 200 {object} User
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /auth/users/{id} [patch]
func (h *Handler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user ID is required"})
		return
	}

	// Get authenticated user ID from context (set by auth middleware)
	authUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Users can only update their own account
	if authUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: cannot update another user's account"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update user
	user, err := h.service.UpdateUser(c.Request.Context(), userID, req)
	if err != nil {
		log.Printf("Failed to update user %s: %v", userID, err)

		// Handle specific errors
		switch err {
		case ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case ErrUsernameExists:
			c.JSON(http.StatusConflict, gin.H{
				"error":   "username_taken",
				"message": "This username is already in use",
				"field":   "username",
			})
		case ErrEmailExists:
			c.JSON(http.StatusConflict, gin.H{
				"error":   "email_taken",
				"message": "This email is already registered",
				"field":   "email",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		}
		return
	}

	c.JSON(http.StatusOK, user)
}

// RequestDeleteCode handles POST /users/:id/request-delete-code
// @Summary Request deletion verification code
// @Description Sends a verification code to user's email for account deletion
// @Tags auth
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /auth/users/{id}/request-delete-code [get]
func (h *Handler) RequestDeleteCode(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user ID is required"})
		return
	}

	// Get authenticated user ID from context
	authUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Users can only request deletion code for their own account
	if authUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: cannot delete another user's account"})
		return
	}

	// Get user to retrieve email
	user, err := h.service.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Failed to get user %s: %v", userID, err)
		if err == ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process request"})
		}
		return
	}

	// Request verification code
	err = h.service.RequestCode(c.Request.Context(), user.Email)
	if err != nil {
		log.Printf("Failed to request delete code for %s: %v", user.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send verification code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "verification code sent to your email",
	})
}

// DeleteUser handles POST /users/:id/delete
// @Summary Delete user account
// @Description Deletes user account after verifying code
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body DeleteUserRequest true "Verification code"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /auth/users/{id}/delete [post]
func (h *Handler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user ID is required"})
		return
	}

	// Get authenticated user ID from context
	authUserID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Users can only delete their own account
	if authUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: cannot delete another user's account"})
		return
	}

	var req DeleteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user email
	user, err := h.service.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		log.Printf("Failed to get user %s: %v", userID, err)
		if err == ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process request"})
		}
		return
	}

	// Delete user (verifies code internally)
	err = h.service.DeleteUser(c.Request.Context(), userID, user.Email, req.Code)
	if err != nil {
		log.Printf("Failed to delete user %s: %v", userID, err)

		switch err {
		case ErrInvalidCode:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid verification code"})
		case ErrUserNotFound:
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		case ErrUnauthorized:
			c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		}
		return
	}

	// Delete session
	sessionID, err := c.Cookie("session_id")
	if err == nil {
		if err := h.sessionMgr.Delete(c.Request.Context(), sessionID); err != nil {
			log.Printf("Failed to delete session %s: %v", sessionID, err)
		}
	}

	// Clear cookie
	c.SetCookie("session_id", "", -1, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"message": "account deleted successfully",
	})
}
