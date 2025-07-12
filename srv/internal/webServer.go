package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"github.com/tristanbatchler/youtube_night/srv/internal/middleware"
	"github.com/tristanbatchler/youtube_night/srv/internal/states"
	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
	"github.com/tristanbatchler/youtube_night/srv/internal/templates"
	"github.com/tristanbatchler/youtube_night/srv/internal/websocket"

	"google.golang.org/api/youtube/v3"

	"golang.org/x/crypto/bcrypt"
)

const AppName = "YouTube Night"

type server struct {
	logger               *log.Logger
	port                 int
	httpServer           *http.Server
	sessionStore         *stores.SessionStore
	userStore            *stores.UserStore
	gangStore            *stores.GangStore
	videoSubmissionStore *stores.VideoSubmissionStore
	youtubeService       *youtube.Service
	wsHub                *websocket.Hub
	gameStateManager     *states.GameStateManager
}

func NewWebServer(port int, logger *log.Logger, sessionStore *stores.SessionStore, userStore *stores.UserStore, gangStore *stores.GangStore, videoSubmissionStore *stores.VideoSubmissionStore, youtubeService *youtube.Service, wsHub *websocket.Hub) (*server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if sessionStore == nil {
		return nil, fmt.Errorf("sessionStore cannot be nil")
	}
	if userStore == nil {
		return nil, fmt.Errorf("userStore cannot be nil")
	}
	if gangStore == nil {
		return nil, fmt.Errorf("gangStore cannot be nil")
	}
	if videoSubmissionStore == nil {
		return nil, fmt.Errorf("videoSubmissionStore cannot be nil")
	}
	if youtubeService == nil {
		return nil, fmt.Errorf("youtubeService cannot be nil")
	}
	if wsHub == nil {
		return nil, fmt.Errorf("wsHub cannot be nil")
	}

	srv := &server{
		logger:               logger,
		port:                 port,
		sessionStore:         sessionStore,
		userStore:            userStore,
		gangStore:            gangStore,
		videoSubmissionStore: videoSubmissionStore,
		youtubeService:       youtubeService,
		wsHub:                wsHub,
		gameStateManager:     states.NewGameStateManager(logger),
	}
	return srv, nil
}

func (s *server) Start() error {
	s.logger.Printf("Starting server on port %d", s.port)

	var stopChan chan os.Signal

	router := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("./srv/static"))
	router.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	loggingMiddleware := middleware.Chain(middleware.Logging, middleware.ContentType)
	redirectIfAuthMiddleware := middleware.RedirectIfAuthenticated(s.logger, s.sessionStore, "/game")
	publicMiddleware := middleware.Chain(loggingMiddleware, redirectIfAuthMiddleware)

	router.Handle("GET /", publicMiddleware(http.HandlerFunc(s.homeHandler)))
	router.Handle("GET /terms", loggingMiddleware(http.HandlerFunc(s.tosHandler)))
	router.Handle("GET /privacy", loggingMiddleware(http.HandlerFunc(s.privacyHandler)))

	router.Handle("GET /join", publicMiddleware(http.HandlerFunc(s.joinPageHandler)))
	router.Handle("POST /join", publicMiddleware(http.HandlerFunc(s.joinActionHandler)))
	router.Handle("GET /host", publicMiddleware(http.HandlerFunc(s.hostPageHandler)))
	router.Handle("POST /host", publicMiddleware(http.HandlerFunc(s.hostActionHandler)))
	router.Handle("GET /gangs/search", publicMiddleware(http.HandlerFunc(s.searchGangsHandler)))

	// SEO routes - no auth middleware needed
	router.Handle("GET /sitemap.xml", middleware.Logging(http.HandlerFunc(s.sitemapHandler)))
	router.Handle("GET /robots.txt", middleware.Logging(http.HandlerFunc(s.robotsHandler)))

	// Protected routes that require authentication
	authMiddleware := middleware.Auth(s.logger, s.sessionStore, s.userStore, s.gangStore)
	protectedMiddleware := middleware.Chain(middleware.Logging, middleware.ContentType, authMiddleware)
	router.Handle("GET /ws", protectedMiddleware(http.HandlerFunc(s.websocketHandler)))
	router.Handle("POST /game/start", protectedMiddleware(http.HandlerFunc(s.startGameHandler)))
	router.Handle("POST /game/stop", protectedMiddleware(http.HandlerFunc(s.stopGameHandler)))
	router.Handle("GET /game", protectedMiddleware(http.HandlerFunc(s.gameHandler)))
	router.Handle("GET /lobby", protectedMiddleware(http.HandlerFunc(s.lobbyHandler)))
	router.Handle("POST /logout", protectedMiddleware(http.HandlerFunc(s.logoutHandler)))
	router.Handle("GET /logout", protectedMiddleware(http.HandlerFunc(s.logoutHandler)))
	router.Handle("GET /videos/search", protectedMiddleware(http.HandlerFunc(s.searchVideosHandler)))
	router.Handle("POST /videos/submit", protectedMiddleware(http.HandlerFunc(s.submitVideoHandler)))
	router.Handle("POST /videos/remove", protectedMiddleware(http.HandlerFunc(s.removeVideoHandler)))

	// Add this route with the protected middleware
	router.Handle("GET /game/change-video", protectedMiddleware(http.HandlerFunc(s.changeVideoHandler)))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: router,
	}

	stopChan = make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine so it doesn't block
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error when running server: %s", err)
		}
	}()

	// Wait for a signal to stop the server
	<-stopChan

	// Shutdown the server gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Error when shutting down server: %v", err)
		return err
	}
	return nil
}

// A helper function to determine whether a request was made by HTMX, so we can use this to inform
// whether the response should be a full layout page or just the partial content
func isHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// A helper function to respond with a template, either as a full page or just the partial content
// depending on whether the request was made by HTMX and the HTML verb used (full pages only apply
// to GET requests) the AppName to the title provided. If the template fails to render, a 500 error
// is returned.
func renderTemplate(w http.ResponseWriter, r *http.Request, t templ.Component, statusCode int, title ...string) {
	w.WriteHeader(statusCode)

	// Return a partial response if the request was made by HTMX or if the request was not a GET request
	if isHtmxRequest(r) || r.Method != http.MethodGet {
		t.Render(r.Context(), w)
		return
	}

	// Otherwise, format the title
	if len(title) <= 0 {
		title = append(title, AppName)
	} else {
		title[0] = fmt.Sprintf("%s ~ %s", title[0], AppName)
	}

	// and render the full page
	err := templates.Layout(t, title[0]).Render(r.Context(), w)
	if err != nil {
		log.Printf("Error when rendering: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *server) homeHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Home(), http.StatusOK, "Home")
}

func (s *server) tosHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.ToS(), http.StatusOK, "Terms of Service")
}

func (s *server) privacyHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Privacy(), http.StatusOK, "Privacy Policy")
}

func (s *server) joinPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Join(), http.StatusOK, "Join")
}

func (s *server) joinActionHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Println("Join action handler called")
	if err := r.ParseForm(); err != nil {
		s.logger.Printf("Error parsing form: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	validationErrors := make([]string, 0)

	formGangName := r.FormValue("gangName")
	if formGangName == "" {
		s.logger.Println("Gang name is required")
		validationErrors = append(validationErrors, "Gang name is required")
	}
	s.logger.Printf("Join action for gang name: %s", formGangName)

	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	gang, err := s.gangStore.GetGangByName(ctx, formGangName)
	if err != nil {
		s.logger.Printf("Error retrieving gang by name: %v", err)

		switch err.(type) {
		case *stores.ErrGangNotFound:
			s.logger.Printf("Gang '%s' not found", formGangName)
			validationErrors = append(validationErrors, "Gang not found")
		case *stores.ErrGangNameInvalid:
			s.logger.Printf("Gang name '%s' is invalid", formGangName)
			validationErrors = append(validationErrors, "Gang name is invalid")
		default:
			s.logger.Printf("Error retrieving gang: %v", err)
			http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
			return
		}
	}
	s.logger.Printf("Gang found: %v", gang)

	// Check if they got the password right
	formGangEntryPassword := r.FormValue("gangEntryPassword")
	if formGangEntryPassword == "" {
		s.logger.Println("Gang entry password is required")
		validationErrors = append(validationErrors, "Gang entry password is required")
	}
	err = bcrypt.CompareHashAndPassword([]byte(gang.EntryPasswordHash), []byte(formGangEntryPassword))

	if err == bcrypt.ErrMismatchedHashAndPassword {
		s.logger.Printf("Gang entry password is incorrect for gang: %s", gang.Name)
		validationErrors = append(validationErrors, "Gang entry password is incorrect")
	} else if err != nil {
		s.logger.Printf("Error comparing gang entry password: %v", err)
		http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
		return
	}

	// Get name from form
	name := r.FormValue("name")
	if name == "" {
		s.logger.Println("Name is required")
		validationErrors = append(validationErrors, "Name is required")
	}

	// Get avatar from form or use default
	avatar := r.FormValue("avatar")
	if avatar == "" {
		s.logger.Println("No avatar selected, using default")
		avatar = "default"
	}

	if len(validationErrors) > 0 {
		s.logger.Printf("Validation errors: %v", validationErrors)
		renderTemplate(w, r, templates.ValidationErrors(validationErrors), http.StatusUnprocessableEntity)
		return
	}

	s.logger.Printf("Gang entry password is correct for gang: %s", gang.Name)

	// Create a new user for this session
	ctx, cancel = context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()

	// Create the user unless one already exists with the same name and is associated with the same gang the user is trying to join right now
	// If the user already exists and is associated with the gang, but has a different avatar, we will update the avatar
	user := db.User{}
	sameNameUsersInGang, err := s.userStore.GetUsersByNameAndGangId(ctx, name, gang.ID)
	if err != nil {
		s.logger.Printf("Error retrieving users by name and gang ID: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if len(sameNameUsersInGang) > 0 {
		// User already exists with the same name in the gang
		s.logger.Printf("User with name '%s' already exists in gang '%s'", name, gang.Name)
		user = sameNameUsersInGang[0]
		// Check if the avatar is different
		if user.AvatarPath.String != avatar {
			s.logger.Printf("Updating avatar for user '%s' in gang '%s'", user.Name, gang.Name)
			// Update the avatar for the existing user
			err = s.userStore.UpdateUserAvatar(ctx, user.ID, avatar)
			if err != nil {
				s.logger.Printf("Error updating user avatar: %v - will just not worry about it", err)
			}
			s.logger.Printf("Using existing user '%s' with ID %d in gang '%s'", user.Name, user.ID, gang.Name)
		}
	} else {
		// Create a new user
		s.logger.Printf("Creating new user with name '%s' and avatar '%s' for gang '%s'", name, avatar, gang.Name)
		ctx, cancel = context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		user, err = s.userStore.CreateUser(ctx, db.CreateUserParams{
			Name:       name,
			AvatarPath: pgtype.Text{String: avatar, Valid: true},
		})
		if err != nil {
			s.logger.Printf("Error creating user: %v", err)
			http.Error(w, "Error creating user", http.StatusInternalServerError)
			return
		}
		s.logger.Printf("Created new user '%s' with ID %d", user.Name, user.ID)

		// Associate the user with the gang
		ctx, cancel = context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		err = s.userStore.AssociateUserWithGang(ctx, user, gang)

		var gangAlreadyExistsError *stores.UserAlreadyInGangError
		if err != nil && !errors.As(err, &gangAlreadyExistsError) {
			s.logger.Printf("Error associating user with gang: %v", err)
			http.Error(w, "Error joining gang", http.StatusInternalServerError)
			return
		}
	}

	isHost, err := s.userStore.IsUserHostOfGang(ctx, user.ID, gang.ID)
	if err != nil {
		s.logger.Printf("Error checking if user is host of gang: %v", err)
		http.Error(w, "Failed to check gang host status", http.StatusInternalServerError)
		return
	}

	// Create a session for the user
	middleware.CreateSessionCookie(w, user.ID, gang.ID, gang.Name, user.Name, avatar, isHost)
	s.logger.Printf("Successfully joined gang: %s", gang.Name)

	// Update the user's last login time
	ctx, cancel = context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	err = s.userStore.UpdateUserLastLogin(ctx, user.ID)
	if err != nil {
		s.logger.Printf("Error updating user last login time: %v", err)
	}

	// Instead of redirecting to home, redirect to game
	http.Redirect(w, r, "/game", http.StatusSeeOther)
}

func (s *server) hostPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Host(), http.StatusOK, "Host")
}

func (s *server) hostActionHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Println("Host action handler called")
	if err := r.ParseForm(); err != nil {
		s.logger.Printf("Error parsing form: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	validationErrors := make([]string, 0)

	formHostName := r.FormValue("hostName")
	if formHostName == "" {
		s.logger.Println("Host name is required")
		validationErrors = append(validationErrors, "Host name is required")
	}

	formAvatar := r.FormValue("avatar")
	if formAvatar == "" {
		s.logger.Println("Avatar is required")
		validationErrors = append(validationErrors, "Host avatar is required")
	}

	formGangName := r.FormValue("gangName")
	if formGangName == "" {
		s.logger.Println("Gang name is required")
		validationErrors = append(validationErrors, "Gang name is required")
	}
	s.logger.Printf("Host action for host name: %s, avatar: %s, gang name: %s", formHostName, formAvatar, formGangName)

	formGangEntryPassword := r.FormValue("gangEntryPassword")
	if formGangEntryPassword == "" {
		s.logger.Println("Gang entry password is required")
		validationErrors = append(validationErrors, "Gang entry password is required")
	}

	formGangEntryPasswordConfirm := r.FormValue("gangEntryPasswordConfirm")
	if formGangEntryPasswordConfirm == "" {
		s.logger.Println("Gang entry password confirmation is required")
		validationErrors = append(validationErrors, "Gang entry password confirmation is required")
	} else if formGangEntryPassword != formGangEntryPasswordConfirm {
		s.logger.Println("Gang entry passwords do not match")
		validationErrors = append(validationErrors, "Gang entry passwords do not match")
	}

	if len(validationErrors) > 0 {
		renderTemplate(w, r, templates.ValidationErrors(validationErrors), http.StatusUnprocessableEntity)
		return
	}

	passwordHashBytes, err := bcrypt.GenerateFromPassword([]byte(formGangEntryPassword), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Printf("Error hashing gang entry password: %v", err)
		http.Error(w, "Error hashing gang entry password", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	user, err := s.userStore.CreateUser(ctx, db.CreateUserParams{
		Name:       formHostName,
		AvatarPath: pgtype.Text{String: formAvatar, Valid: true},
	})
	if err != nil {
		s.logger.Printf("Error creating user: %v", err)
		http.Error(w, "Error creating host user", http.StatusInternalServerError)
		return
	}

	ctx, cancel = context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	gang, err := s.gangStore.CreateGang(ctx, formGangName, user.ID, string(passwordHashBytes))
	if err != nil {
		switch err.(type) {
		case *stores.ErrGangNameAlreadyExists:
			s.logger.Printf("Gang name '%s' already exists", formGangName)
			validationErrors = append(validationErrors, "Gang name already exists")
		case *stores.ErrGangNameInvalid:
			s.logger.Printf("Gang name '%s' is invalid", formGangName)
			validationErrors = append(validationErrors, "Gang name is invalid")
		default:
			s.logger.Printf("Error creating gang: %v", err)
			http.Error(w, "Error creating gang", http.StatusInternalServerError)
			return
		}
	}

	if len(validationErrors) > 0 {
		renderTemplate(w, r, templates.ValidationErrors(validationErrors), http.StatusUnprocessableEntity)
		return
	}

	s.logger.Printf("Host action successful: user %v created and gang %v created", user, gang)

	// Get the host to join the gang they just created
	middleware.CreateSessionCookie(w, user.ID, gang.ID, gang.Name, user.Name, formAvatar, true)
	ctx, cancel = context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	err = s.userStore.UpdateUserLastLogin(ctx, user.ID)
	if err != nil {
		s.logger.Printf("Error updating user last login time: %v", err)
	}
	http.Redirect(w, r, "/lobby", http.StatusSeeOther)
}

func (s *server) searchGangsHandler(w http.ResponseWriter, r *http.Request) {
	// Get search query from the parameters
	query := r.URL.Query().Get("gangName")
	s.logger.Printf("Searching gangs with query: %s", query)

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	gangs, err := s.gangStore.SearchGangs(ctx, query)
	if err != nil {
		s.logger.Printf("Error searching gangs: %v", err)
		http.Error(w, "Error searching gangs", http.StatusInternalServerError)
		return
	}
	s.logger.Printf("Found %d gangs matching query '%s'", len(gangs), query)
	renderTemplate(w, r, templates.GangsList(gangs), http.StatusOK)
}

func (s *server) lobbyHandler(w http.ResponseWriter, r *http.Request) {
	// Get session data
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if this gang is current in an active game, and redirect to the game if so
	if s.gameStateManager.IsGameActive(sessionData.GangId) {
		s.logger.Printf("Gang ID %d is currently in an active game, redirecting to game page", sessionData.GangId)
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}

	s.logger.Printf("Loading videos submitted for gang ID %d and user ID %d", sessionData.GangId, sessionData.UserId)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	videoList, err := s.videoSubmissionStore.GetVideosSubmittedByGangIdAndUserId(ctx, sessionData.UserId, sessionData.GangId)
	if err != nil {
		s.logger.Printf("Error fetching video details: %v", err)
		http.Error(w, "Failed to load video details", http.StatusInternalServerError)
		return
	}
	s.logger.Printf("Loaded %d videos for gang ID %d", len(videoList), sessionData.GangId)

	renderTemplate(w, r, templates.Lobby(videoList, sessionData), http.StatusOK, "Lobby")
}

func (s *server) gameHandler(w http.ResponseWriter, r *http.Request) {
	// Get session data
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if the game has actually started, or redirect to the lobby
	gameStarted := s.gameStateManager.IsGameActive(sessionData.GangId)

	if !gameStarted {
		s.logger.Println("Game has not started yet, redirecting to lobby")
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	gameState, exists := s.gameStateManager.GetGameState(sessionData.GangId)
	if !exists {
		s.logger.Println("No active game state found")
		http.Error(w, "No active game state found", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, r, templates.Game(gameState, sessionData), http.StatusOK, "Game")
}

func (s *server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	// If you're the host of an active game, stop the game
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if sessionData.IsHost && s.gameStateManager.IsGameActive(sessionData.GangId) {
		s.logger.Printf("User %d is host of gang %d, stopping active game before logout", sessionData.UserId, sessionData.GangId)
		err := s.shutdownGame(sessionData)
		if err != nil {
			s.logger.Printf("Error stopping game: %v", err)
		}
	}

	// Delete the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
	})

	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusSeeOther)
	s.logger.Println("User logged out successfully, session cookie cleared")
}

func (s *server) searchVideosHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Search query parameter q is required", http.StatusBadRequest)
		return
	}

	s.logger.Printf("Searching YouTube videos with query: %s", query)

	// Set up the search call
	call := s.youtubeService.Search.List([]string{"snippet"}).
		Q(query).
		MaxResults(5).
		Type("video")

	// Execute the search
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	response, err := call.Context(ctx).Do()
	if err != nil {
		s.logger.Printf("YouTube search error: %v", err)
		http.Error(w, "Error searching YouTube", http.StatusInternalServerError)
		return
	}

	// Render the search results
	renderTemplate(w, r, templates.VideoSearchResults(response.Items), http.StatusOK)
}

func (s *server) submitVideoHandler(w http.ResponseWriter, r *http.Request) {
	// Get the video details
	video := db.Video{
		VideoID:      r.FormValue("videoId"),
		Title:        r.FormValue("title"),
		Description:  r.FormValue("description"),
		ThumbnailUrl: r.FormValue("thumbnailUrl"),
		ChannelName:  r.FormValue("channelName"),
	}

	s.logger.Printf("Submitting video %v", video)

	// Get the session data
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the userId and gangId from the session
	userId := sessionData.UserId
	gangId := sessionData.GangId

	// Add the video submission to the store
	_, err := s.videoSubmissionStore.SubmitVideo(r.Context(), video, userId, gangId)
	if err != nil {
		s.logger.Printf("Error submitting video: %v", err)
		http.Error(w, "Error submitting video", http.StatusInternalServerError)
		return
	}

	// Get updated count after submission for the counter
	videos, err := s.videoSubmissionStore.GetVideosSubmittedByGangIdAndUserId(
		r.Context(), userId, gangId)
	if err != nil {
		s.logger.Printf("Error getting video count: %v", err)
	}

	// Use the template component instead of direct HTML generation
	w.WriteHeader(http.StatusOK)
	err = templates.SubmitVideoResponse(video, len(videos)).Render(r.Context(), w)
	if err != nil {
		s.logger.Printf("Error rendering video submit response template: %v", err)
	}
}

func (s *server) removeVideoHandler(w http.ResponseWriter, r *http.Request) {
	// Get the video ID from the form data
	videoId := r.FormValue("videoId")
	if videoId == "" {
		http.Error(w, "Video ID is required", http.StatusBadRequest)
		return
	}
	s.logger.Printf("Removing video with ID: %s", videoId)

	// Get the session data
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the userId and gangId from the session
	userId := sessionData.UserId
	gangId := sessionData.GangId

	// Remove the video submission from the store
	err := s.videoSubmissionStore.RemoveVideoSubmission(r.Context(), videoId, userId, gangId)
	if err != nil {
		s.logger.Printf("Error removing video: %v", err)
		http.Error(w, "Error removing video", http.StatusInternalServerError)
		return
	}

	// Get the updated video count
	videos, err := s.videoSubmissionStore.GetVideosSubmittedByGangIdAndUserId(
		r.Context(), userId, gangId)
	if err != nil {
		s.logger.Printf("Error getting updated video count: %v", err)
	}

	// Use the template component instead of direct HTML generation
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	err = templates.RemoveVideoResponse(videoId, videos).Render(r.Context(), w)
	if err != nil {
		s.logger.Printf("Error rendering video remove response template: %v", err)
	}
}

func (s *server) websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Get session data
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if the user is a host
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	isHost, err := s.userStore.IsUserHostOfGang(ctx, sessionData.UserId, sessionData.GangId)
	if err != nil {
		s.logger.Printf("Error checking if user is host: %v", err)
		// Continue even if there's an error, assume they're not a host
		isHost = false
	}

	// Serve WebSocket connection
	websocket.ServeWs(s.wsHub, w, r, sessionData.UserId, sessionData.GangId, isHost)
}

func (s *server) startGameHandler(w http.ResponseWriter, r *http.Request) {
	// Verify the user is authorized
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if the user is the host
	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	isHost, err := s.userStore.IsUserHostOfGang(ctx, sessionData.UserId, sessionData.GangId)
	if err != nil {
		s.logger.Printf("Error checking if user is host: %v", err)
		http.Error(w, "Error checking host status", http.StatusInternalServerError)
		return
	}

	if !isHost {
		http.Error(w, "Only the host can start the game", http.StatusForbidden)
		return
	}

	// Get all videos submitted to this gang
	ctx, cancel = context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	allVideos, err := s.videoSubmissionStore.GetAllVideosInGang(ctx, sessionData.GangId)
	if err != nil {
		s.logger.Printf("Error getting all videos in gang: %v", err)
		http.Error(w, "Error retrieving videos", http.StatusInternalServerError)
		return
	}

	numVids := len(allVideos)
	s.logger.Printf("Starting game for gang ID %d with %d videos", sessionData.GangId, numVids)

	// First shuffle the videos so that the game is fair
	shuffledVideos := make([]db.Video, 0, numVids)
	seenIndices := make(map[int]struct{})
	for len(shuffledVideos) < numVids {
		i := rand.IntN(numVids)
		if _, seen := seenIndices[i]; !seen {
			seenIndices[i] = struct{}{}
			shuffledVideos = append(shuffledVideos, allVideos[i])
		}
	}

	s.gameStateManager.StartGame(sessionData.GangId, shuffledVideos)

	s.logger.Printf("Sending game start message to gang ID %d with %d videos", sessionData.GangId, numVids)
	websocket.SendGameStart(s.wsHub, sessionData.GangId)

	// Return success
	w.WriteHeader(http.StatusOK)
	response := struct {
		Success bool `json:"success"`
		Count   int  `json:"videoCount"`
	}{
		Success: true,
		Count:   numVids,
	}

	json.NewEncoder(w).Encode(response)
}

func (s *server) shutdownGame(sessionData *stores.SessionData) error {
	// Check if the user is the host
	if !sessionData.IsHost {
		return fmt.Errorf("only the host can stop the game")
	}

	// Check if the game is actually running
	if !s.gameStateManager.IsGameActive(sessionData.GangId) {
		return fmt.Errorf("no active game to stop for gang ID %d", sessionData.GangId)
	}

	s.logger.Printf("Stopping game for gang ID %d", sessionData.GangId)
	s.gameStateManager.StopGame(sessionData.GangId)

	s.logger.Printf("Sending game stop message to gang ID %d", sessionData.GangId)
	websocket.SendGameStop(s.wsHub, sessionData.GangId)

	return nil
}

func (s *server) stopGameHandler(w http.ResponseWriter, r *http.Request) {
	// Verify the user is authorized
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	err := s.shutdownGame(sessionData)
	if err != nil {
		s.logger.Printf("Error stopping game: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	response := struct {
		Success bool `json:"success"`
	}{
		Success: true,
	}
	json.NewEncoder(w).Encode(response)
}

func (s *server) sitemapHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, host)

	// Static page URLs
	urls := []struct {
		Loc        string
		LastMod    string
		ChangeFreq string
		Priority   string
	}{
		{baseURL + "/", time.Now().Format("2006-01-02"), "weekly", "1.0"},
		{baseURL + "/join", time.Now().Format("2006-01-02"), "weekly", "0.8"},
		{baseURL + "/host", time.Now().Format("2006-01-02"), "weekly", "0.8"},
		{baseURL + "/terms", time.Now().Format("2006-01-02"), "monthly", "0.5"},
		{baseURL + "/privacy", time.Now().Format("2006-01-02"), "monthly", "0.5"},
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)

	// Write XML header and opening tags
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

	// Write each URL entry
	for _, url := range urls {
		fmt.Fprintf(w, `
  <url>
    <loc>%s</loc>
    <lastmod>%s</lastmod>
    <changefreq>%s</changefreq>
    <priority>%s</priority>
  </url>`, url.Loc, url.LastMod, url.ChangeFreq, url.Priority)
	}

	// Close the urlset tag
	fmt.Fprintf(w, `
</urlset>`)
}

func (s *server) robotsHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", scheme, host)

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `User-agent: *
Allow: /
Allow: /join
Allow: /host
Allow: /terms
Allow: /privacy

# Disallow authenticated pages
Disallow: /lobby
Disallow: /game
Disallow: /ws
Disallow: /logout

# Disallow API endpoints
Disallow: /videos/search
Disallow: /videos/submit
Disallow: /gangs/search

# Point to sitemap
Sitemap: %s
`, sitemapURL)
}

// changeVideoHandler processes a request to change the currently playing video
func (s *server) changeVideoHandler(w http.ResponseWriter, r *http.Request) {
	// Get session data to verify permissions
	sessionData, ok := middleware.GetSessionData(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only hosts can change videos
	if !sessionData.IsHost {
		http.Error(w, "Only hosts can change videos", http.StatusForbidden)
		return
	}

	// Get video details from query params
	videoID := r.URL.Query().Get("videoId")
	indexStr := r.URL.Query().Get("index")

	if videoID == "" {
		http.Error(w, "Video ID is required", http.StatusBadRequest)
		return
	}

	// Parse index as integer
	index := 0
	if indexStr != "" {
		var err error
		index, err = strconv.Atoi(indexStr)
		if err != nil {
			s.logger.Printf("Error parsing index: %v", err)
			http.Error(w, "Invalid index", http.StatusBadRequest)
			return
		}
	}

	// Get the game state to access video details
	gameState, exists := s.gameStateManager.GetGameState(sessionData.GangId)
	if !exists {
		http.Error(w, "No active game", http.StatusBadRequest)
		return
	}

	// Find the video in the game state
	var title, channel string
	if index >= 0 && index < len(gameState.Videos) {
		title = gameState.Videos[index].Title
		channel = gameState.Videos[index].ChannelName
	} else {
		s.logger.Printf("Video index out of range: %d", index)
		http.Error(w, "Video index out of range", http.StatusBadRequest)
		return
	}

	// Broadcast the video change to all clients in the gang
	websocket.SendVideoChange(s.wsHub, sessionData.GangId, videoID, index, title, channel)

	// Return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
