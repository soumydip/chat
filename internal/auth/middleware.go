package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	ProfileCacheTTL = 15 * time.Minute
	SessionCacheTTL = 5 * time.Minute
)

var (
	ErrUserNotFound = errors.New("user not found")

	cookieSecure = os.Getenv("COOKIE_SECURE") == "true"
)

type TrimmedUser struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	IsDeleted bool   `json:"is_deleted"`
}

func SetAuthCookies(c *gin.Context, accessToken, refreshToken string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", accessToken, 15*60, "/", "", cookieSecure, true)
	if refreshToken != "" {
		c.SetCookie("refresh_token", refreshToken, 30*24*60*60, "/", "", cookieSecure, true)
	}
}

func ClearAuthCookies(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", "", -1, "/", "", cookieSecure, true)
	c.SetCookie("refresh_token", "", -1, "/", "", cookieSecure, true)
}

func InvalidateProfileCache(ctx context.Context, rdb *redis.Client, userID string) {
	_ = rdb.Del(ctx, fmt.Sprintf("auth:profile:%s", userID)).Err()
}

func InvalidateSessionCache(ctx context.Context, rdb *redis.Client, sessionID string) {
	_ = rdb.Del(ctx, fmt.Sprintf("auth:session:%s", sessionID)).Err()
}

func getOrFetchUserProfile(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, userID string) (*TrimmedUser, error) {
	cacheKey := fmt.Sprintf("auth:profile:%s", userID)
	cachedData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var user TrimmedUser
		if json.Unmarshal([]byte(cachedData), &user) == nil {
			return &user, nil // cache hit
		}
	}

	query := `
		SELECT id, username, full_name, COALESCE(is_deleted, false)
		FROM users
		WHERE id = $1
		LIMIT 1;
	`
	var user TrimmedUser
	err = db.QueryRow(ctx, query, userID).Scan(&user.UserID, &user.Username, &user.FullName, &user.IsDeleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// Asynchronously save to Redis
	go func(u TrimmedUser) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if data, err := json.Marshal(u); err == nil {
			_ = rdb.Set(bgCtx, cacheKey, string(data), ProfileCacheTTL).Err()
		}
	}(user)

	return &user, nil
}

func isSessionActive(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, sessionID, userID string) (bool, error) {
	cacheKey := fmt.Sprintf("auth:session:%s", sessionID)
	if v, err := rdb.Get(ctx, cacheKey).Result(); err == nil && v == userID {
		return true, nil
	}

	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_sessions
			WHERE id = $1 AND user_id = $2 AND expires_at > NOW()
		);
	`, sessionID, userID).Scan(&exists)
	if err != nil {
		return false, err
	}

	if exists {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = rdb.Set(bgCtx, cacheKey, userID, SessionCacheTTL).Err()
		}()
	}
	return exists, nil
}

func abort(c *gin.Context, status int, msg string, clearCookies bool) {
	if clearCookies {
		ClearAuthCookies(c)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// main middleware function to validate JWT and handle auto-refresh
func ValidateJWT(db *pgxpool.Pool, rdb *redis.Client, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		// get access token from cookie or Authorization header
		accessToken, err := c.Cookie("access_token")
		if err != nil || accessToken == "" {
			authHeader := c.GetHeader("Authorization")
			if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
				accessToken = strings.TrimSpace(authHeader[7:])
			}
		}

		refreshToken, _ := c.Cookie("refresh_token")

		if accessToken != "" {
			token, parseErr := jwt.Parse(
				accessToken,
				func(t *jwt.Token) (interface{}, error) { return []byte(jwtSecret), nil },
				jwt.WithValidMethods([]string{"HS256"}),
				jwt.WithExpirationRequired(),
			)

			if parseErr == nil && token.Valid {
				if claims, ok := token.Claims.(jwt.MapClaims); ok {
					userID, _ := claims["user_id"].(string)
					sessionID, _ := claims["session_id"].(string)

					if userID != "" && sessionID != "" {
						active, sessErr := isSessionActive(ctx, db, rdb, sessionID, userID)
						if sessErr != nil {
							log.Printf("session check failed: %v", sessErr)
							abort(c, http.StatusInternalServerError, "Internal error", false)
							return
						}

						if active {
							profile, profileErr := getOrFetchUserProfile(ctx, db, rdb, userID)
							if profileErr != nil {
								if errors.Is(profileErr, ErrUserNotFound) {
									abort(c, http.StatusUnauthorized, "Invalid session. Please login again.", true)
								} else {
									log.Printf("profile fetch failed: %v", profileErr)
									abort(c, http.StatusInternalServerError, "Internal error", false)
								}
								return
							}

							if profile.IsDeleted {
								abort(c, http.StatusForbidden, "Account deleted", true)
								return
							}

							c.Set("user", profile)
							c.Set("user_id", profile.UserID)
							c.Next()
							return
						}
					}
				}
			}
		}

		if refreshToken == "" {
			abort(c, http.StatusUnauthorized, "Session expired. Please login again.", true)
			return
		}

		var sessionUserID, sessionID string
		err = db.QueryRow(ctx, `
			SELECT user_id, id
			FROM user_sessions
			WHERE session_token = $1 AND expires_at > NOW()
			LIMIT 1;
		`, HashToken(refreshToken)).Scan(&sessionUserID, &sessionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				abort(c, http.StatusUnauthorized, "Invalid or expired session. Please login again.", true)
			} else {
				log.Printf("session lookup failed: %v", err)
				abort(c, http.StatusInternalServerError, "Internal error", false)
			}
			return
		}

		profile, err := getOrFetchUserProfile(ctx, db, rdb, sessionUserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				abort(c, http.StatusUnauthorized, "Profile not found", true)
			} else {
				log.Printf("profile fetch failed: %v", err)
				abort(c, http.StatusInternalServerError, "Internal error", false)
			}
			return
		}

		if profile.IsDeleted {
			abort(c, http.StatusForbidden, "Account deleted", true)
			return
		}

		newAccessToken, err := GenerateAccessToken(profile.UserID, profile.Username, sessionID)
		if err != nil {
			log.Printf("Failed to generate refreshed access token: %v", err)
			abort(c, http.StatusInternalServerError, "Failed to refresh token", false)
			return
		}

		SetAuthCookies(c, newAccessToken, "")

		c.Set("user", profile)
		c.Set("user_id", profile.UserID)
		c.Next()
	}
}
