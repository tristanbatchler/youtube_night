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

	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"github.com/tristanbatchler/youtube_night/srv/internal/middleware"
	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
	"github.com/tristanbatchler/youtube_night/srv/internal/templates"
)

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

	router.Handle("GET /", middleware.Logging(http.HandlerFunc(s.homeHandler)))

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

func (s *server) homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	users, err := s.userStore.GetUsers(ctx)
	if err != nil {
		s.logger.Printf("Error retrieving users: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	component := templates.Home(users)
	err = templates.Layout(component, "YouTube Night").Render(r.Context(), w)
	if err != nil {
		s.logger.Printf("Error rendering home page: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
