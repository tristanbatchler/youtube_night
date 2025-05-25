package stores

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

type UserStore struct {
	queries *db.Queries
	logger  *log.Logger
}

func NewUserStore(queries *db.Queries, logger *log.Logger) (*UserStore, error) {
	if queries == nil {
		return nil, fmt.Errorf("queries cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	return &UserStore{
		queries: queries,
		logger:  logger,
	}, nil
}

type UserAlreadyExistsError struct {
	Username string
}

func (e *UserAlreadyExistsError) Error() string {
	return fmt.Sprintf("user with username '%s' already exists", e.Username)
}

func (us *UserStore) CreateUser(ctx context.Context, params db.CreateUserParams) (db.User, error) {
	emptyUser := db.User{}

	if params.Username == "" {
		return emptyUser, fmt.Errorf("username cannot be empty")
	}

	params.Username = strings.TrimSpace(strings.ToLower(params.Username))

	user, err := us.queries.CreateUser(ctx, params)
	if db.ErrorHasCode(err, pgerrcode.UniqueViolation) {
		return emptyUser, &UserAlreadyExistsError{Username: params.Username}
	}
	return user, nil
}

func (us *UserStore) GetUsers(ctx context.Context) ([]db.User, error) {
	users, err := us.queries.GetUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("error retrieving users: %w", err)
	}
	return users, nil
}
