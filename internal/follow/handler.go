package follow

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Follow handles POST /follow
// @Summary Follow a user
// @Description Follow a user by user ID (requires authentication)
// @Tags follow
// @Accept json
// @Produce json
// @Param follow body FollowRequest true "Follow request data"
// @Success 201 {object} Follow
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /api/follow [post]
func (h *Handler) Follow(c *gin.Context) {
	followerID := c.GetHeader("X-User-ID")
	if followerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req FollowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	f, err := h.svc.Follow(c, followerID, req.FolloweeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	c.JSON(http.StatusCreated, f)
}

// Unfollow handles DELETE /follow/{user_id}
// @Summary Unfollow a user
// @Description Unfollow a user by user ID (requires authentication)
// @Tags follow
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /api/follow/{user_id} [delete]
func (h *Handler) Unfollow(c *gin.Context) {
	followerID := c.GetHeader("X-User-ID")
	if followerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	followeeID := c.Param("user_id")

	_, err := h.svc.Unfollow(c, followerID, followeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// FollowersCount handles GET /follow/{user_id}/followers/count
// @Summary Get followers count
// @Description Get followers count for a user
// @Tags follow
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} CountResponse
// @Failure 500 {object} map[string]string
// @Router /api/follow/{user_id}/followers/count [get]
func (h *Handler) FollowersCount(c *gin.Context) {
	userID := c.Param("user_id")

	cnt, err := h.svc.FollowersCount(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	c.JSON(http.StatusOK, CountResponse{UserID: userID, Count: cnt})
}

// FollowingCount handles GET /follow/{user_id}/following/count
// @Summary Get following count
// @Description Get following count for a user
// @Tags follow
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} CountResponse
// @Failure 500 {object} map[string]string
// @Router /api/follow/{user_id}/following/count [get]
func (h *Handler) FollowingCount(c *gin.Context) {
	userID := c.Param("user_id")

	cnt, err := h.svc.FollowingCount(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	c.JSON(http.StatusOK, CountResponse{UserID: userID, Count: cnt})
}

// IsFollowing handles GET /follow/{user_id}/following/me
// @Summary Check follow status
// @Description Check if current user follows the specified user
// @Tags follow
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} FollowingResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security SessionAuth
// @Router /api/follow/{user_id}/following/me [get]
func (h *Handler) IsFollowing(c *gin.Context) {
	followerID := c.GetHeader("X-User-ID")
	if followerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	followeeID := c.Param("user_id")

	ok, err := h.svc.IsFollowing(c, followerID, followeeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	c.JSON(http.StatusOK, FollowingResponse{UserID: followeeID, Following: ok})
}

// Health handles GET /health
// @Summary Health check
// @Description Follow service health check
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "follow-service",
	})
}
