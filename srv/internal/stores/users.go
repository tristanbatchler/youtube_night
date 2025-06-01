package stores

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

type UserStore struct {
	dbPool  *pgxpool.Pool
	queries *db.Queries
	logger  *log.Logger
}

func NewUserStore(dbPool *pgxpool.Pool, logger *log.Logger) (*UserStore, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("dbPool cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	return &UserStore{
		dbPool:  dbPool,
		queries: db.New(dbPool),
		logger:  logger,
	}, nil
}

type UserAlreadyInGangError struct {
	Name     string
	GangName string
}

func (e *UserAlreadyInGangError) Error() string {
	return fmt.Sprintf("user '%s' is already in gang '%s'", e.Name, e.GangName)
}

func (us *UserStore) CreateUser(ctx context.Context, params db.CreateUserParams) (db.User, error) {
	emptyUser := db.User{}

	if params.Name == "" {
		return emptyUser, fmt.Errorf("name cannot be empty")
	}

	if !params.AvatarPath.Valid {
		params.AvatarPath = pgtype.Text{String: "cat", Valid: true}
	}

	params.Name = strings.TrimSpace(params.Name)

	user, err := us.queries.CreateUser(ctx, params)
	if err != nil {
		return emptyUser, fmt.Errorf("error creating user: %w", err)
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

func (us *UserStore) AssociateUserWithGang(ctx context.Context, user db.User, gang db.Gang) error {
	others, err := us.queries.GetUsersInGang(ctx, gang.ID)
	if err != nil {
		return fmt.Errorf("error retrieving users in gang: %w", err)
	}

	// Make sure only one user with a certain name and avatar is in this gang
	for _, other := range others {
		if other.Name == user.Name && other.AvatarPath == user.AvatarPath {
			return &UserAlreadyInGangError{
				Name:     user.Name,
				GangName: gang.Name,
			}
		}
	}

	err = us.queries.AssociateUserWithGang(ctx, db.AssociateUserWithGangParams{
		UserID: user.ID,
		GangID: gang.ID,
	})
	if err != nil {
		return fmt.Errorf("error associating user with gang: %w", err)
	}
	return nil
}

func (us *UserStore) GetUserById(ctx context.Context, userId int32) (db.User, error) {
	user, err := us.queries.GetUserById(ctx, userId)
	if err != nil {
		return db.User{}, fmt.Errorf("error retrieving user by ID: %w", err)
	}
	return user, nil
}

func (us *UserStore) GetUsersByNameAndGangId(ctx context.Context, name string, gangId int32) ([]db.User, error) {
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}
	if gangId <= 0 {
		return nil, fmt.Errorf("gangId must be a positive integer")
	}

	users, err := us.queries.GetUsersByNameAndGangId(ctx, db.GetUsersByNameAndGangIdParams{
		Name:   name,
		GangID: gangId,
	})
	if err != nil {
		return nil, fmt.Errorf("error retrieving users by name and gang ID: %w", err)
	}
	return users, nil
}

func (us *UserStore) UpdateUserAvatar(ctx context.Context, userId int32, avatarPath string) error {
	if userId <= 0 {
		return fmt.Errorf("userId must be a positive integer")
	}
	if avatarPath == "" {
		return fmt.Errorf("avatarPath cannot be empty")
	}

	err := us.queries.UpdateUserAvatar(ctx, db.UpdateUserAvatarParams{
		ID:         userId,
		AvatarPath: pgtype.Text{String: avatarPath, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("error updating user avatar: %w", err)
	}
	return nil
}

func (us *UserStore) UpdateUserLastLogin(ctx context.Context, userId int32) error {
	if userId <= 0 {
		return fmt.Errorf("userId must be a positive integer")
	}

	err := us.queries.UpdateUserLastLogin(ctx, userId)
	if err != nil {
		return fmt.Errorf("error updating user last login: %w", err)
	}
	return nil
}

func (us *UserStore) IsUserHostOfGang(ctx context.Context, userId int32, gangId int32) (bool, error) {
	if userId <= 0 {
		return false, fmt.Errorf("userId must be a positive integer")
	}
	if gangId <= 0 {
		return false, fmt.Errorf("gangId must be a positive integer")
	}

	isHost, err := us.queries.IsUserHostOfGang(ctx, db.IsUserHostOfGangParams{
		UserID: userId,
		GangID: gangId,
	})
	if err != nil {
		return false, fmt.Errorf("error checking if user is host of gang: %w", err)
	}
	return isHost, nil
}
