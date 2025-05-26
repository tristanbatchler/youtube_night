package internal

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tristanbatchler/youtube_night/srv/internal/db"
	"github.com/tristanbatchler/youtube_night/srv/internal/middleware"
	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
	"github.com/tristanbatchler/youtube_night/srv/internal/templates"

	"golang.org/x/crypto/bcrypt"
)

const AppName = "YouTube Night"

type server struct {
	logger     *log.Logger
	port       int
	httpServer *http.Server
	userStore  *stores.UserStore
	gangStore  *stores.GangStore
}

func NewWebServer(port int, logger *log.Logger, userStore *stores.UserStore, gangStore *stores.GangStore) (*server, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if userStore == nil {
		return nil, fmt.Errorf("userStore cannot be nil")
	}
	if gangStore == nil {
		return nil, fmt.Errorf("gangStore cannot be nil")
	}

	srv := &server{
		logger:    logger,
		port:      port,
		userStore: userStore,
		gangStore: gangStore,
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

	router.Handle("GET /", loggingMiddleware(http.HandlerFunc(s.homeHandler)))
	router.Handle("GET /join", loggingMiddleware(http.HandlerFunc(s.joinPageHandler)))
	router.Handle("POST /join", loggingMiddleware(http.HandlerFunc(s.joinActionHandler)))
	router.Handle("GET /host", loggingMiddleware(http.HandlerFunc(s.hostPageHandler)))
	router.Handle("POST /host", loggingMiddleware(http.HandlerFunc(s.hostActionHandler)))

	// Search gangs
	router.Handle("GET /gangs/search", loggingMiddleware(http.HandlerFunc(s.searchGangsHandler)))

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
	formGangName := r.FormValue("gangName")
	if formGangName == "" {
		s.logger.Println("Gang name is required")
		http.Error(w, "Gang name is required", http.StatusBadRequest)
		return
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
			http.Error(w, "Gang not found", http.StatusUnprocessableEntity)
		case *stores.ErrGangNameInvalid:
			s.logger.Printf("Gang name '%s' is invalid", formGangName)
			http.Error(w, "Gang name is invalid", http.StatusUnprocessableEntity)
		default:
			s.logger.Printf("Error retrieving gang: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}
	s.logger.Printf("Gang found: %v", gang)

	// Check if they got the password right
	formGangEntryPassword := r.FormValue("gangEntryPassword")
	if formGangEntryPassword == "" {
		s.logger.Println("Gang entry password is required")
		http.Error(w, "Gang entry password is required", http.StatusBadRequest)
		return
	}
	err = bcrypt.CompareHashAndPassword([]byte(gang.EntryPasswordHash), []byte(formGangEntryPassword))
	if err != nil {
		s.logger.Printf("Error comparing gang entry password: %v", err)
		http.Error(w, "Invalid gang entry password", http.StatusUnauthorized)
		return
	}
	s.logger.Printf("Gang entry password is correct for gang: %s", gang.Name)

	// Print a success message
	s.logger.Printf("Successfully joined gang: %s", gang.Name)

	// Redirect back to the home page for now
	renderTemplate(w, r, templates.Home(), http.StatusOK, "Home - Join Successful")
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

	formHostAvatar := r.FormValue("hostAvatar")
	if formHostAvatar == "" {
		s.logger.Println("Host avatar is required")
		validationErrors = append(validationErrors, "Host avatar is required")
	}

	formGangName := r.FormValue("gangName")
	if formGangName == "" {
		s.logger.Println("Gang name is required")
		validationErrors = append(validationErrors, "Gang name is required")
	}
	s.logger.Printf("Host action for host name: %s, avatar: %s, gang name: %s", formHostName, formHostAvatar, formGangName)

	formGangEntryPassword := r.FormValue("gangEntryPassword")
	if formGangEntryPassword == "" {
		s.logger.Println("Gang entry password is required")
		validationErrors = append(validationErrors, "Gang entry password is required")
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
		AvatarPath: pgtype.Text{String: formHostAvatar, Valid: true},
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
	renderTemplate(w, r, templates.Home(), http.StatusOK, "Home - Host Successful")
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
