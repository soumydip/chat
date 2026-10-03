package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var ErrEmailTaken = errors.New("email already registered")

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	FullName     string    `json:"full_name"`
	Email        string    `json:"email,omitempty"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Repository struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewRepository(db *pgxpool.Pool, redis *redis.Client) *Repository {
	return &Repository{db: db, redis: redis}
}

func (r *Repository) setCacheAsync(key, value string, ttl time.Duration) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := r.redis.Set(bgCtx, key, value, ttl).Err(); err != nil {
			log.Printf("Async redis set failed for key %s: %v", key, err)
		}
	}()
}

// GetUserByUsername: username আগে থেকে আছে কিনা
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (bool, error) {
	cacheKey := "user:exists:" + username

	cachedVal, err := r.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return cachedVal == "1", nil
	} else if !errors.Is(err, redis.Nil) {
		log.Printf("Redis error for key %s: %v", cacheKey, err)
	}

	var id string
	err = r.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1 LIMIT 1;`, username).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.setCacheAsync(cacheKey, "0", 1*time.Minute)
			return false, nil
		}
		return false, err
	}

	r.setCacheAsync(cacheKey, "1", 30*time.Minute)
	return true, nil
}

func (r *Repository) CreateUser(ctx context.Context, username, fullName, email, passwordHash string) (*User, error) {
	id := CreateNewUUIDV7()

	_, err := r.db.Exec(ctx, `
		INSERT INTO users (id, username, full_name, email, password_hash)
		VALUES ($1, $2, $3, $4, $5);`,
		id, username, fullName, email, passwordHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "email") {
				return nil, ErrEmailTaken
			}
			return nil, ErrUsernameTaken
		}
		return nil, err
	}

	// stale "0" cache মুছে দেওয়ার জন্য
	_ = r.redis.Set(ctx, "user:exists:"+username, "1", 30*time.Minute).Err()

	return &User{ID: id, Username: username, FullName: fullName, Email: email}, nil
}

func (r *Repository) CreateSession(ctx context.Context, id, userID, tokenHash string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, session_token, expires_at)
		VALUES ($1, $2, $3, NOW() + INTERVAL '30 days');`,
		id, userID, tokenHash)
	return err
}

func (r *Repository) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`DELETE FROM user_sessions WHERE session_token = $1 RETURNING id;`,
		tokenHash).Scan(&id)
	return id, err
}

func (r *Repository) GetuserForLogin(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx, `
		SELECT id, username, full_name, password_hash
		FROM users
		WHERE email = $1 AND COALESCE(is_deleted, false) = false
		LIMIT 1;`, email,
	).Scan(&user.ID, &user.Username, &user.FullName, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound // middleware file-এ আগেই declare করা আছে
		}
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx,
		`SELECT id, username, full_name, created_at FROM users WHERE id = $1 LIMIT 1;`, userID,
	).Scan(&user.ID, &user.Username, &user.FullName, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}
