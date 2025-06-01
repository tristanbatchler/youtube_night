package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/tristanbatchler/youtube_night/srv/internal/stores"
)

const (
	SessionCookieName = "youtube_night_session"
	SessionExpiration = 24 * time.Hour
)

// UserContextKey is used to store user data in the request context
type UserContextKey string

const UserKey UserContextKey = "user"

// Auth creates a middleware that validates session cookies and redirects unauthenticated users
func Auth(logger *log.Logger, sessionStore *stores.SessionStore, userStore *stores.UserStore, gangStore *stores.GangStore) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for session cookie
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				logger.Printf("No session cookie found: %v", err)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}

			// Get session from cookie value
			sessionToken := cookie.Value
			if sessionToken == "" {
				logger.Println("Empty session token")
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}

			// Validate the session token using the session store
			sessionData, valid, err := sessionStore.ValidateToken(sessionToken)
			if err != nil {
				logger.Printf("Error validating session: %v", err)
				clearSessionAndRedirect(w, r)
				return
			}

			if !valid {
				logger.Println("Invalid session token")
				clearSessionAndRedirect(w, r)
				return
			}

			// If you want to fetch the latest user data
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()

			// Optional: Check if user still exists in database
			user, err := userStore.GetUserById(ctx, int32(sessionData.UserId))
			if err != nil {
				logger.Printf("User from session not found: %v", err)
				clearSessionAndRedirect(w, r)
				return
			}

			// Optional: Check if gang still exists
			gang, err := gangStore.GetGangById(ctx, int32(sessionData.GangId))
			if err != nil {
				logger.Printf("Gang from session not found: %v", err)
				clearSessionAndRedirect(w, r)
				return
			}

			// Add session data to the request context
			ctx = context.WithValue(r.Context(), UserKey, sessionData)

			// Update the session data if needed (like adding more user details from DB)
			sessionData.Name = user.Name
			sessionData.GangName = gang.Name

			// Call the next handler with the enriched context
			next.ServeHTTP(w, r.WithContext(ctx))

			// Optionally rotate the session token periodically
			if sessionStore.ShouldRotateToken(sessionToken) {
				newToken, err := sessionStore.RotateToken(sessionToken, sessionData)
				if err == nil {
					setSessionCookie(w, newToken)
				}
			}
		})
	}
}

// RedirectIfAuthenticated redirects users to the game if they're already authenticated
func RedirectIfAuthenticated(logger *log.Logger, sessionStore *stores.SessionStore, endpoint string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for session cookie
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				// No session cookie, continue to requested page
				next.ServeHTTP(w, r)
				return
			}

			// Get session from cookie value
			sessionToken := cookie.Value
			if sessionToken == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Validate the session token
			sessionData, valid, err := sessionStore.ValidateToken(sessionToken)
			if err != nil || !valid || sessionData == nil {
				// Invalid session, clear the cookie and continue
				clearSessionAndRedirect(w, r)
				return
			}

			// User is authenticated, redirect to game
			logger.Printf("Authenticated user accessing %s, redirecting to game", r.URL.Path)
			http.Redirect(w, r, endpoint, http.StatusSeeOther)
		})
	}
}

func clearSessionAndRedirect(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Create a session cookie for the authenticated user
func CreateSessionCookie(w http.ResponseWriter, userId int32, gangId int32, gangName string, name string, avatar string, isHost bool) {
	// Create the session data
	sessionData := &stores.SessionData{
		UserId:    userId,
		GangId:    gangId,
		GangName:  gangName,
		Name:      name,
		Avatar:    avatar,
		IsHost:    isHost,
		CreatedAt: time.Now().Unix(),
		Expiry:    time.Now().Add(SessionExpiration).Unix(),
	}

	// Get the global session store (you might want to inject this instead)
	sessionStore := stores.GetSessionStore()

	// Generate a token
	token, err := sessionStore.CreateToken(sessionData)
	if err != nil {
		log.Printf("Error creating session token: %v", err)
		return
	}

	// Set the cookie
	setSessionCookie(w, token)
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(SessionExpiration),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// Get session data from request context
func GetSessionData(r *http.Request) (*stores.SessionData, bool) {
	// First try getting it from context (if Auth middleware was used)
	ctx := r.Context()
	if sessionData, ok := ctx.Value(UserKey).(*stores.SessionData); ok {
		return sessionData, true
	}

	// Fallback to getting it from cookie
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, false
	}

	sessionStore := stores.GetSessionStore()
	sessionData, valid, err := sessionStore.ValidateToken(cookie.Value)
	if err != nil || !valid {
		return nil, false
	}

	return sessionData, true
}
