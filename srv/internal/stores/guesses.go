package stores

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

// GuessStore handles operations related to video guesses
type GuessStore struct {
	dbPool  *pgxpool.Pool
	queries *db.Queries
	logger  *log.Logger
}

// NewGuessStore creates a new guess store
func NewGuessStore(dbPool *pgxpool.Pool, logger *log.Logger) (*GuessStore, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("dbPool cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	return &GuessStore{
		dbPool:  dbPool,
		queries: db.New(dbPool),
		logger:  logger,
	}, nil
}

// RecordGuess records or updates a user's guess for a video
func (gs *GuessStore) RecordGuess(ctx context.Context, userID, gangID int32, videoID string, guessedUserID int32) (db.VideoGuess, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	guess, err := gs.queries.CreateVideoGuess(ctx, db.CreateVideoGuessParams{
		UserID:        userID,
		GangID:        gangID,
		VideoID:       videoID,
		GuessedUserID: guessedUserID,
	})

	if err != nil {
		return db.VideoGuess{}, fmt.Errorf("error recording guess: %w", err)
	}

	return guess, nil
}

// GetUserGuessForVideo returns a user's guess for a specific video
func (gs *GuessStore) GetUserGuessForVideo(ctx context.Context, userID, gangID int32, videoID string) (db.VideoGuess, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	guess, err := gs.queries.GetVideoGuessForUser(ctx, db.GetVideoGuessForUserParams{
		UserID:  userID,
		GangID:  gangID,
		VideoID: videoID,
	})

	if err != nil {
		return db.VideoGuess{}, fmt.Errorf("error getting user guess: %w", err)
	}

	return guess, nil
}

// GetAllGuessesForVideo returns all guesses for a specific video in a gang
func (gs *GuessStore) GetAllGuessesForVideo(ctx context.Context, gangID int32, videoID string) ([]db.GetAllGuessesForVideoRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	guesses, err := gs.queries.GetAllGuessesForVideo(ctx, db.GetAllGuessesForVideoParams{
		GangID:  gangID,
		VideoID: videoID,
	})

	if err != nil {
		return nil, fmt.Errorf("error getting all guesses: %w", err)
	}

	return guesses, nil
}

// GetVideoSubmitter returns the user who submitted a specific video
func (gs *GuessStore) GetVideoSubmitter(ctx context.Context, gangID int32, videoID string) (db.GetVideoSubmitterRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	submitter, err := gs.queries.GetVideoSubmitter(ctx, db.GetVideoSubmitterParams{
		GangID:  gangID,
		VideoID: videoID,
	})

	if err != nil {
		return db.GetVideoSubmitterRow{}, fmt.Errorf("error getting video submitter: %w", err)
	}

	return submitter, nil
}

// DeleteGuessesForGang deletes all guesses for a specific gang (used when stopping a game)
func (gs *GuessStore) DeleteGuessesForGang(ctx context.Context, gangID int32) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := gs.queries.DeleteGuessesForGang(ctx, gangID)
	if err != nil {
		return fmt.Errorf("error deleting guesses for gang: %w", err)
	}

	return nil
}
