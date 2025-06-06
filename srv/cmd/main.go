package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/tristanbatchler/youtube_night/srv/internal"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
	"github.com/tristanbatchler/youtube_night/srv/internal/websocket"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type config struct {
	PgHost         string
	PgPort         int
	PgUser         string
	PgPassword     string
	PgDatabase     string
	WebPort        int
	SessionToken   []byte
	YtApiClientKey string
}

func loadConfig() (*config, error) {
	err := godotenv.Load("srv/.env")
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %v", err)
	}

	cfg := &config{
		PgHost:         os.Getenv("PG_HOST"),
		PgPort:         5432, // Default PostgreSQL port
		PgUser:         os.Getenv("PG_USER"),
		PgPassword:     os.Getenv("PG_PASSWORD"),
		PgDatabase:     os.Getenv("PG_DATABASE"),
		WebPort:        9000, // Default web server port
		SessionToken:   []byte(os.Getenv("SESSION_TOKEN")),
		YtApiClientKey: os.Getenv("YT_API_KEY"),
	}

	if len(cfg.SessionToken) == 0 {
		return nil, fmt.Errorf("SESSION_TOKEN environment variable is required")
	}

	if cfg.YtApiClientKey == "" {
		return nil, fmt.Errorf("YT_API_KEY environment variable is required")
	}

	if cfg.PgHost == "" || cfg.PgUser == "" || cfg.PgPassword == "" || cfg.PgDatabase == "" {
		return nil, fmt.Errorf("missing required environment variables for PostgreSQL configuration")
	}

	if pgPortStr, found := os.LookupEnv("PG_PORT"); found {
		pgPort, err := strconv.Atoi(pgPortStr)
		if err != nil {
			return nil, fmt.Errorf("invalid PG_PORT value: %v", err)
		}
		cfg.PgPort = pgPort
	}
	if webPortStr, found := os.LookupEnv("WEB_PORT"); found {
		webPort, err := strconv.Atoi(webPortStr)
		if err != nil {
			return nil, fmt.Errorf("invalid WEB_PORT value: %v", err)
		}
		cfg.WebPort = webPort
	}
	return cfg, nil
}

func main() {
	logger := log.New(os.Stdout, "[Main] ", log.LstdFlags)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("Error loading configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	youtubeService, err := youtube.NewService(ctx, option.WithAPIKey(cfg.YtApiClientKey))
	if err != nil {
		logger.Fatalf("Error creating YouTube service: %v", err)
	}

	pgConnString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.PgHost, cfg.PgPort, cfg.PgUser, cfg.PgPassword, cfg.PgDatabase,
	)

	dbPool, err := pgxpool.New(ctx, pgConnString)
	if err != nil {
		logger.Fatalf("Error connecting to PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	logger.Printf("Connected to PostgreSQL database %s at %s:%d", cfg.PgDatabase, cfg.PgHost, cfg.PgPort)

	if err := db.GenSchema(dbPool); err != nil {
		logger.Fatalf("Error generating database schema: %v", err)
	}
	logger.Println("Database schema generated successfully")

	sessionStore := stores.NewSessionStore(cfg.SessionToken)

	userStore, err := stores.NewUserStore(dbPool, logger)
	if err != nil {
		logger.Fatalf("Error creating user store: %v", err)
	}

	gangStore, err := stores.NewGangStore(dbPool, logger)
	if err != nil {
		logger.Fatalf("Error creating gang store: %v", err)
	}

	videoSubmissionStore, err := stores.NewVideoSubmissionStore(youtubeService, dbPool, logger)
	if err != nil {
		logger.Fatalf("Error creating video submission store: %v", err)
	}

	wsHub := websocket.NewHub(logger)
	go wsHub.Run()

	webServer, err := internal.NewWebServer(cfg.WebPort, logger, sessionStore, userStore, gangStore, videoSubmissionStore, youtubeService, wsHub)
	if err != nil {
		logger.Fatalf("Error creating web server: %v", err)
	}
	if err := webServer.Start(); err != nil {
		logger.Fatalf("Error starting web server: %v", err)
	}
	logger.Printf("Server started successfully on port %d", cfg.WebPort)
}
