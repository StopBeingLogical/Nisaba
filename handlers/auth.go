package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "nisaba_session"
	sessionHMACPrefix = "v1."
)

// AuthMiddleware protects routes. Valid session cookie → pass through.
// HTMX requests without a session → 401 + HX-Trigger to show the login modal
// (no browser dialog). Full-page requests without a session → 401 plain text.
func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}

		// HTMX request: tell the client to show the login modal instead of
		// the browser's native Basic Auth dialog.
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Trigger", "showLoginModal")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Full-page navigation to a protected route: redirect to home with
		// ?login=1 so the modal opens automatically.
		http.Redirect(w, r, "/?login=1", http.StatusSeeOther)
	})
}

// Login accepts a form POST with username + password, issues a session cookie
// on success, and returns an HTMX-friendly response.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.Header().Set("HX-Trigger", `{"loginError":"Invalid request."}`)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username != "bobby" || !h.checkPassword(password) {
		w.Header().Set("HX-Trigger", `{"loginError":"Incorrect username or password."}`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	h.issueSessionCookie(w)
	// Tell the client: close the modal, then refresh the page so the
	// protected action the user originally attempted becomes available.
	w.Header().Set("HX-Trigger", "loginSuccess")
	w.WriteHeader(http.StatusOK)
}

// Logout clears the session cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Redirect to home; the session is gone so protected pages will show the modal.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// validSession returns true if the request carries a valid, unexpired session cookie.
func (h *Handler) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	secret, err := h.sessionSecret()
	if err != nil {
		return false
	}
	return verifySessionToken(cookie.Value, secret)
}

// checkPassword verifies a plaintext password against the stored bcrypt hash.
func (h *Handler) checkPassword(password string) bool {
	hash, err := h.store.GetConfig("auth.password_hash")
	if err != nil || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// issueSessionCookie sets a signed session cookie on the response.
func (h *Handler) issueSessionCookie(w http.ResponseWriter) {
	secret, err := h.sessionSecret()
	if err != nil {
		return
	}
	hours, _ := h.store.GetConfig("auth.session_hours")
	duration := 12 * time.Hour
	if h, err := strconv.Atoi(hours); err == nil && h > 0 {
		duration = time.Duration(h) * time.Hour
	}

	token := newSessionToken(secret, duration)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(duration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionSecret returns the HMAC signing key, auto-generating it on first use.
func (h *Handler) sessionSecret() ([]byte, error) {
	secret, err := h.store.GetConfig("auth.secret")
	if err != nil {
		return nil, err
	}
	if secret == "" {
		secret, err = generateSecret()
		if err != nil {
			return nil, err
		}
		if err := h.store.SetConfig("auth.secret", secret); err != nil {
			return nil, err
		}
	}
	return []byte(secret), nil
}

// newSessionToken creates a signed token: "v1.{expiry_unix}.{hmac_hex}"
func newSessionToken(secret []byte, duration time.Duration) string {
	expiry := strconv.FormatInt(time.Now().Add(duration).Unix(), 10)
	mac := tokenMAC(secret, expiry)
	return sessionHMACPrefix + expiry + "." + mac
}

// verifySessionToken checks signature and expiry.
func verifySessionToken(token string, secret []byte) bool {
	if !strings.HasPrefix(token, sessionHMACPrefix) {
		return false
	}
	rest := token[len(sessionHMACPrefix):]
	dot := strings.LastIndexByte(rest, '.')
	if dot < 0 {
		return false
	}
	expiry := rest[:dot]
	gotMAC := rest[dot+1:]

	// Constant-time MAC comparison.
	expectedMAC := tokenMAC(secret, expiry)
	if subtle.ConstantTimeCompare([]byte(gotMAC), []byte(expectedMAC)) != 1 {
		return false
	}

	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}

func tokenMAC(secret []byte, data string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}
