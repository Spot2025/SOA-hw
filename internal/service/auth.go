package service

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"marketplace/internal/apperror"
	"marketplace/internal/config"
	"marketplace/internal/repository"
)

type AuthService struct {
	userRepo *repository.UserRepo
	cfg      *config.Config
}

func NewAuthService(userRepo *repository.UserRepo, cfg *config.Config) *AuthService {
	return &AuthService{userRepo: userRepo, cfg: cfg}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
	Role         string
}

func (s *AuthService) Register(ctx context.Context, username, password, role string) (*TokenPair, error) {
	existing, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperror.UsernameConflict()
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &repository.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return s.generateTokens(user.ID, user.Role)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperror.InvalidCredentials()
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, apperror.InvalidCredentials()
	}

	return s.generateTokens(user.ID, user.Role)
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	token, err := jwt.Parse(refreshToken, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, apperror.RefreshTokenInvalid()
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, apperror.RefreshTokenInvalid()
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		return nil, apperror.RefreshTokenInvalid()
	}

	userID := int64(claims["user_id"].(float64))
	role := claims["role"].(string)

	return s.generateTokens(userID, role)
}

func (s *AuthService) generateTokens(userID int64, role string) (*TokenPair, error) {
	now := time.Now()

	accessClaims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"type":    "access",
		"exp":     now.Add(s.cfg.JWTAccessTTL).Unix(),
		"iat":     now.Unix(),
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	refreshClaims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"type":    "refresh",
		"exp":     now.Add(s.cfg.JWTRefreshTTL).Unix(),
		"iat":     now.Unix(),
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       userID,
		Role:         role,
	}, nil
}
