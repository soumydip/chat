package auth

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUsernameTaken      = errors.New("username already taken")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type RegisterInput struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	FullName string `json:"full_name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=64"`
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*User, error) {
	if err := ValidatePasswordStrength(input.Password); err != nil {
		return nil, err
	}

	taken, err := s.repo.GetUserByUsername(ctx, input.Username)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, ErrUsernameTaken
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user, err := s.repo.CreateUser(ctx, input.Username, input.FullName, input.Email, string(hashedPassword))
	if err != nil {
		return nil, err
	}

	user.PasswordHash = ""
	return user, nil
}

type LoginInput struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*User, error) {
	user, err := s.repo.GetuserForLogin(ctx, input.Email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	user.PasswordHash = ""
	return user, nil
}

// tokenHash = HashToken(rawRefreshToken). Returns the new session ID.
func (s *Service) CreateSession(ctx context.Context, userID, tokenHash string) (string, error) {
	sessionID := CreateNewUUIDV7()

	if err := s.repo.CreateSession(ctx, sessionID, userID, tokenHash); err != nil {
		return "", err
	}
	return sessionID, nil
}

// Logout: raw refresh token দিলে session মুছে session ID ফেরত দেয় (cache invalidate করার জন্য)
func (s *Service) RevokeSession(ctx context.Context, rawRefreshToken string) (string, error) {
	return s.repo.DeleteSessionByTokenHash(ctx, HashToken(rawRefreshToken))
}

type CheckUsernameInput struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
}

func (s *Service) CheckUserNameExists(ctx context.Context, input CheckUsernameInput) (bool, error) {
	return s.repo.GetUserByUsername(ctx, input.Username)
}
