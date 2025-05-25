package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/a-h/templ"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"github.com/tristanbatchler/youtube_night/srv/internal/middleware"
	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
	"github.com/tristanbatchler/youtube_night/srv/internal/templates"
)

const AppName = "YouTube Night"

type server struct {
	logger     *log.Logger
	port       int
	httpServer *http.Server
	userStore  *stores.UserStore
}

func NewWebServer(port int, logger *log.Logger, userStore *stores.UserStore) (*server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	srv := &server{
		logger:    logger,
		port:      port,
		userStore: userStore,
	}
	return srv, nil
}

func (s *server) Start() error {
	s.logger.Printf("Starting server on port %d", s.port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := s.userStore.CreateUser(ctx, db.CreateUserParams{
		Username:     "admin",
		PasswordHash: "123",
	})
	var alreadyExistsErr *stores.UserAlreadyExistsError
	if err != nil && !errors.As(err, &alreadyExistsErr) {
		s.logger.Printf("Error creating default user: %v", err)
	} else {
		s.logger.Printf("Default user created successfully: %v", user)
	}

	var stopChan chan os.Signal

	router := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("./srv/static"))
	router.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	loggingMiddleware := middleware.Chain(middleware.Logging, middleware.ContentType)

	router.Handle("GET /", loggingMiddleware(http.HandlerFunc(s.homeHandler)))
	router.Handle("GET /join", loggingMiddleware(http.HandlerFunc(s.joinPageHandler)))
	router.Handle("POST /join", loggingMiddleware(http.HandlerFunc(s.joinActionHandler)))
	router.Handle("GET /host", loggingMiddleware(http.HandlerFunc(s.hostPageHandler)))
	router.Handle("POST /host", loggingMiddleware(http.HandlerFunc(s.hostActionHandler)))

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
func renderTemplate(w http.ResponseWriter, r *http.Request, t templ.Component, title ...string) {
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
	// ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	// defer cancel()
	// users, err := s.userStore.GetUsers(ctx)
	// if err != nil {
	// 	s.logger.Printf("Error retrieving users: %v", err)
	// 	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	// 	return
	// }

	renderTemplate(w, r, templates.Home(), "Home")
}

func (s *server) joinPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Join(), "Join")
}

func (s *server) joinActionHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Println("Join action handler called")
	if err := r.ParseForm(); err != nil {
		s.logger.Printf("Error parsing form: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	formGameCode := r.FormValue("gameCode")
	if formGameCode == "" {
		s.logger.Println("Game code is required")
		http.Error(w, "Game code is required", http.StatusBadRequest)
		return
	}
	s.logger.Printf("Join action for game code: %s", formGameCode)
	// Here you would handle the join action, e.g., joining a game with the provided code. For now, just log it and redirect back to home.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) hostPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, r, templates.Host(), "Host")
}

func (s *server) hostActionHandler(w http.ResponseWriter, r *http.Request) {
	s.logger.Println("Host action handler called")
	if err := r.ParseForm(); err != nil {
		s.logger.Printf("Error parsing form: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	formGroupName := r.FormValue("groupName")
	if formGroupName == "" {
		s.logger.Println("Group name is required")
		http.Error(w, "Group name is required", http.StatusBadRequest)
		return
	}
	s.logger.Printf("Host action for group name: %s", formGroupName)
	// Here you would handle the host action, e.g., creating a new group. For now, just log it and redirect back to home.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
