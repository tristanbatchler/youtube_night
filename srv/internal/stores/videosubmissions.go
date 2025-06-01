package stores

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"google.golang.org/api/youtube/v3"
)

type VideoSubmissionStore struct {
	youtubeService *youtube.Service
	dbPool         *pgxpool.Pool
	queries        *db.Queries
	logger         *log.Logger
}

func NewVideoSubmissionStore(youtubeService *youtube.Service, dbPool *pgxpool.Pool, logger *log.Logger) (*VideoSubmissionStore, error) {
	if youtubeService == nil {
		return nil, log.Output(2, "youtubeService cannot be nil")
	}

	if dbPool == nil {
		return nil, log.Output(2, "dbPool cannot be nil")
	}
	if logger == nil {
		return nil, log.Output(2, "logger cannot be nil")
	}
	return &VideoSubmissionStore{
		youtubeService: youtubeService,
		dbPool:         dbPool,
		queries:        db.New(dbPool),
		logger:         logger,
	}, nil
}

func (s *VideoSubmissionStore) SubmitVideo(ctx context.Context, video db.Video, userId int32, gangId int32) (db.VideoSubmission, error) {
	emptySubmission := db.VideoSubmission{}

	if video.VideoID == "" || video.Title == "" || !video.ThumbnailUrl.Valid || !video.Description.Valid || video.ChannelName == "" {
		return emptySubmission, fmt.Errorf("video details are incomplete")
	}
	if userId <= 0 {
		return emptySubmission, fmt.Errorf("userId must be a positive integer")
	}
	if gangId <= 0 {
		return emptySubmission, fmt.Errorf("gangId must be a positive integer")
	}

	tx, err := s.dbPool.Begin(ctx)
	if err != nil {
		return emptySubmission, fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)
	params := db.CreateVideoIfNotExistsParams(video)
	err = qtx.CreateVideoIfNotExists(ctx, params)
	if err != nil {
		return emptySubmission, fmt.Errorf("error creating video record: %w", err)
	}
	submission, err := qtx.CreateVideoSubmission(ctx, db.CreateVideoSubmissionParams{
		VideoID: video.VideoID,
		UserID:  userId,
		GangID:  gangId,
	})
	if err != nil {
		return emptySubmission, fmt.Errorf("error creating video submission: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return emptySubmission, fmt.Errorf("error committing transaction: %w", err)
	}
	return submission, nil
}

func (s *VideoSubmissionStore) RemoveVideoSubmission(ctx context.Context, videoId string, userId int32, gangId int32) error {
	if videoId == "" {
		return fmt.Errorf("videoId cannot be empty")
	}
	if userId <= 0 {
		return fmt.Errorf("userId must be a positive integer")
	}
	if gangId <= 0 {
		return fmt.Errorf("gangId must be a positive integer")
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := s.queries.DeleteVideoSubmission(ctx, db.DeleteVideoSubmissionParams{
		VideoID: videoId,
		UserID:  userId,
		GangID:  gangId,
	})
	if err != nil {
		return fmt.Errorf("error removing video submission for videoId %s: %w", videoId, err)
	}
	return nil
}

func (s *VideoSubmissionStore) GetVideosSubmittedByGangIdAndUserId(ctx context.Context, userId int32, gangId int32) ([]db.Video, error) {
	if gangId <= 0 {
		return nil, fmt.Errorf("gangId must be a positive integer")
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	details, err := s.queries.GetVideosSubmittedByGangIdAndUserId(ctx, db.GetVideosSubmittedByGangIdAndUserIdParams{
		GangID: gangId,
		UserID: userId,
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching video submissions for gangId %d: %w", gangId, err)
	}

	videos := make([]db.Video, 0, len(details))
	for _, detail := range details {
		videos = append(videos, db.Video{
			VideoID:      detail.VideoID,
			Title:        detail.Title,
			Description:  detail.Description,
			ThumbnailUrl: detail.ThumbnailUrl,
			ChannelName:  detail.ChannelName,
		})
	}
	return videos, nil
}
