package internal

import (
	"log"
	"sync"
	"time"

	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

// GameState represents the current state of a game for a specific gang
type GameState struct {
	GangID        int32
	StartedAt     time.Time
	Videos        []db.Video
	PlayerGuesses map[int32]map[string]int32 // Map of userID -> (videoID -> guessed userID)
}

// GameStateManager manages active games
type GameStateManager struct {
	mu          sync.RWMutex
	activeGames map[int32]*GameState // Map of gangID to game state
	logger      *log.Logger
}

// NewGameStateManager creates a new game state manager
func NewGameStateManager(logger *log.Logger) *GameStateManager {
	return &GameStateManager{
		activeGames: make(map[int32]*GameState),
		logger:      logger,
	}
}

// StartGame marks a gang as having an active game
func (g *GameStateManager) StartGame(gangID int32, videos []db.Video) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.activeGames[gangID]; exists {
		g.logger.Printf("Game already started for gang %d", gangID)
		return false
	}

	g.activeGames[gangID] = &GameState{
		GangID:        gangID,
		StartedAt:     time.Now(),
		Videos:        videos,
		PlayerGuesses: make(map[int32]map[string]int32),
	}

	g.logger.Printf("Game started for gang %d with %d videos", gangID, len(videos))
	return true
}

// StopGame marks a gang as no longer having an active game
func (g *GameStateManager) StopGame(gangID int32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.activeGames[gangID]; !exists {
		g.logger.Printf("No active game for gang %d", gangID)
		return false
	}

	delete(g.activeGames, gangID)
	g.logger.Printf("Game stopped for gang %d", gangID)
	return true
}

// IsGameActive checks if a gang has an active game
func (g *GameStateManager) IsGameActive(gangID int32) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	_, exists := g.activeGames[gangID]
	return exists
}

// GetGameVideos returns the videos for an active game
func (g *GameStateManager) GetGameVideos(gangID int32) ([]db.Video, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gameState, exists := g.activeGames[gangID]
	if !exists {
		return nil, false
	}

	return gameState.Videos, true
}

// RecordGuess records a player's guess for a video
func (g *GameStateManager) RecordGuess(gangID int32, userID int32, videoID string, guessedUserID int32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	gameState, exists := g.activeGames[gangID]
	if !exists {
		return false
	}

	if _, exists := gameState.PlayerGuesses[userID]; !exists {
		gameState.PlayerGuesses[userID] = make(map[string]int32)
	}

	gameState.PlayerGuesses[userID][videoID] = guessedUserID
	return true
}

// GetGameState returns the complete game state for a gang
func (g *GameStateManager) GetGameState(gangID int32) (*GameState, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gameState, exists := g.activeGames[gangID]
	if !exists {
		return nil, false
	}

	return gameState, true
}

// GetAllGuesses returns all guesses for a specific gang
func (g *GameStateManager) GetAllGuesses(gangID int32) (map[int32]map[string]int32, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gameState, exists := g.activeGames[gangID]
	if !exists {
		return nil, false
	}

	return gameState.PlayerGuesses, true
}

// GetActiveGamesCount returns the number of active games
func (g *GameStateManager) GetActiveGamesCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.activeGames)
}
