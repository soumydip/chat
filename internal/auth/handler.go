package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) issueSession(c *gin.Context, userID, username string) error {
	ctx := c.Request.Context()

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return err
	}

	sessionID, err := h.service.CreateSession(ctx, userID, HashToken(refreshToken))
	if err != nil {
		return err
	}

	accessToken, err := GenerateAccessToken(userID, username, sessionID)
	if err != nil {
		return err
	}

	SetAuthCookies(c, accessToken, refreshToken)
	return nil
}

func (h *Handler) RegisterUser(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.Register(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrUsernameTaken) || errors.Is(err, ErrEmailTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	if err := h.issueSession(c, user.ID, user.Username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully!",
		"user_id": user.ID,
	})
}

func (h *Handler) LoginUser(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.Login(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Login failed"})
		}
		return
	}

	if err := h.issueSession(c, user.ID, user.Username); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User logged in successfully!",
		"user":    user,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	if refresh, err := c.Cookie("refresh_token"); err == nil && refresh != "" {
		if sessionID, err := h.service.RevokeSession(c.Request.Context(), refresh); err == nil {
			InvalidateSessionCache(c.Request.Context(), h.service.repo.redis, sessionID)
		}
	}
	ClearAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out"})
}

func (h *Handler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User ID not found "})
		return
	}
	user, err := h.service.repo.GetUserByID(c.Request.Context(), userID.(string))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "User retrieved successfully", "user": user})
}
