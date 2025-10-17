package states

import (
	"log"
	"sync"
	"time"

	"github.com/tristanbatchler/youtube_night/srv/internal/db"
)

// GameState represents the current state of a game for a specific gang
type GameState struct {
	GangID      int32
	StartedAt   time.Time
	Videos      []db.Video
	GangMembers []db.User
	Submitters  map[string]int32 // Map of videoID -> submitterID
	mu          sync.RWMutex     // Mutex for thread-safe access
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
func (g *GameStateManager) StartGame(gangID int32, videos []db.Video, members []db.User, submitters map[string]int32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.activeGames[gangID]; exists {
		g.logger.Printf("Game already started for gang %d", gangID)
		return false
	}

	g.activeGames[gangID] = &GameState{
		GangID:      gangID,
		StartedAt:   time.Now(),
		Videos:      videos,
		GangMembers: members,
		Submitters:  submitters,
	}

	g.logger.Printf("Game started for gang %d with %d videos and %d members",
		gangID, len(videos), len(members))
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

// GetSubmitterIDForVideo gets the submitter ID for a video in a gang
func (g *GameStateManager) GetSubmitterIDForVideo(gangID int32, videoID string) (int32, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gameState, exists := g.activeGames[gangID]
	if !exists {
		return -1, false
	}

	gameState.mu.RLock()
	defer gameState.mu.RUnlock()

	submitterID, exists := gameState.Submitters[videoID]
	return submitterID, exists
}

// GetActiveGamesCount returns the number of active games
func (g *GameStateManager) GetActiveGamesCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.activeGames)
}

// GetVideoSubmitter returns the member who submitted a specific video
func (gs *GameState) GetVideoSubmitter(videoID string) (*db.User, bool) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	submitterID, exists := gs.Submitters[videoID]
	if !exists {
		return nil, false
	}

	// Find the member with this ID
	for i := range gs.GangMembers {
		if gs.GangMembers[i].ID == submitterID {
			return &gs.GangMembers[i], true
		}
	}

	return nil, false
}
