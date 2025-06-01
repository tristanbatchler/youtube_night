package stores

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

type GangStore struct {
	dbPool  *pgxpool.Pool
	queries *db.Queries
	logger  *log.Logger
}

type ErrGangNotFound struct {
	GangName string
}

func (e *ErrGangNotFound) Error() string {
	return fmt.Sprintf("gang '%s' not found", e.GangName)
}

type ErrGangNameInvalid struct {
	GangName string
}

func (e *ErrGangNameInvalid) Error() string {
	return fmt.Sprintf("gang name '%s' is invalid", e.GangName)
}

type ErrGangNameAlreadyExists struct {
	GangName string
}

func (e *ErrGangNameAlreadyExists) Error() string {
	return fmt.Sprintf("gang name '%s' already exists", e.GangName)
}

func NewGangStore(dbPool *pgxpool.Pool, logger *log.Logger) (*GangStore, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("dbPool cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	return &GangStore{
		dbPool:  dbPool,
		queries: db.New(dbPool),
		logger:  logger,
	}, nil
}

func (gs *GangStore) CreateGang(ctx context.Context, name string, hostUserId int32, entryPasswordHash string) (db.Gang, error) {
	emptyGang := db.Gang{}

	if name == "" {
		return emptyGang, &ErrGangNameInvalid{GangName: name}
	}

	name = strings.TrimSpace(name)

	tx, err := gs.dbPool.Begin(ctx)
	if err != nil {
		return emptyGang, fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := gs.queries.WithTx(tx)
	gang, err := qtx.CreateGang(ctx, db.CreateGangParams{
		Name:              name,
		EntryPasswordHash: entryPasswordHash,
	})
	if err != nil {
		if db.ErrorHasCode(err, pgerrcode.UniqueViolation) {
			return emptyGang, &ErrGangNameAlreadyExists{GangName: name}
		}
		return emptyGang, fmt.Errorf("error creating gang: %w", err)
	}
	err = qtx.AssociateUserWithGang(ctx, db.AssociateUserWithGangParams{
		UserID: hostUserId,
		GangID: gang.ID,
		Ishost: true,
	})
	if err != nil {
		return emptyGang, fmt.Errorf("error associating user with gang: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return emptyGang, fmt.Errorf("error committing transaction: %w", err)
	}
	return gang, nil
}

func (gs *GangStore) GetGangs(ctx context.Context) ([]db.Gang, error) {
	gangs, err := gs.queries.GetGangs(ctx)
	if err != nil {
		return nil, fmt.Errorf("error retrieving gangs: %w", err)
	}
	return gangs, nil
}

func (gs *GangStore) SearchGangs(ctx context.Context, searchTerm string) ([]db.Gang, error) {
	if searchTerm == "" {
		return gs.GetGangs(ctx)
	}
	gangs, err := gs.queries.SearchGangs(ctx, pgtype.Text{String: searchTerm, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("error searching gangs: %w", err)
	}
	return gangs, nil
}

func (gs *GangStore) GetGangByName(ctx context.Context, name string) (db.Gang, error) {
	emptyGang := db.Gang{}

	if name == "" {
		return emptyGang, &ErrGangNameInvalid{GangName: name}
	}
	gang, err := gs.queries.GetGangByName(ctx, name)
	if err == pgx.ErrNoRows {
		return emptyGang, &ErrGangNotFound{GangName: name}
	} else if err != nil {
		return emptyGang, fmt.Errorf("error retrieving gang by name: %w", err)
	}
	return gang, nil
}

func (gs *GangStore) GetGangById(ctx context.Context, id int32) (db.Gang, error) {
	emptyGang := db.Gang{}

	if id <= 0 {
		return emptyGang, fmt.Errorf("invalid gang ID: %d", id)
	}
	gang, err := gs.queries.GetGangById(ctx, id)
	if err == pgx.ErrNoRows {
		return emptyGang, &ErrGangNotFound{GangName: fmt.Sprintf("ID %d", id)}
	} else if err != nil {
		return emptyGang, fmt.Errorf("error retrieving gang by ID: %w", err)
	}
	return gang, nil
}

func (gs *GangStore) IsGameStarted(ctx context.Context, gangId int32) (bool, error) {
	if gangId <= 0 {
		return false, fmt.Errorf("invalid gang ID: %d", gangId)
	}

	isStarted, err := gs.queries.IsGangCurrentlyInGame(ctx, gangId)
	if err != nil {
		return false, fmt.Errorf("error checking if game is started for gang ID %d: %w", gangId, err)
	}
	return isStarted, nil
}

func (gs *GangStore) SetGameStarted(ctx context.Context, gangId int32, started bool) error {
	if gangId <= 0 {
		return fmt.Errorf("invalid gang ID: %d", gangId)
	}

	err := gs.queries.SetGangCurrentlyInGame(ctx, db.SetGangCurrentlyInGameParams{
		ID:              gangId,
		CurrentlyInGame: started,
	})
	if err != nil {
		return fmt.Errorf("error setting game started for gang ID %d: %w", gangId, err)
	}
	return nil
}
