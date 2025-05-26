package stores

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

type GangStore struct {
	dbPool  *pgxpool.Pool
	queries *db.Queries
	logger  *log.Logger
}

func NewGangStore(dbPool *pgxpool.Pool, logger *log.Logger) (*GangStore, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("dbTx cannot be nil")
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
		return emptyGang, fmt.Errorf("name cannot be empty")
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
		return emptyGang, fmt.Errorf("error creating gang: %w", err)
	}
	err = qtx.AssociateUserWithGang(ctx, db.AssociateUserWithGangParams{
		UserID: hostUserId,
		GangID: gang.ID,
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
