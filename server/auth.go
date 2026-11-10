package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName   = "biodentis_session"
	sessionIdle  = 30 * time.Minute
	sessionLimit = 12 * time.Hour
)

type loginSession struct {
	username  string
	idleUntil time.Time
	expiresAt time.Time
}

func validPassword(password string) bool {
	return len(password) >= 12 && len(password) <= 72
}

func sessionCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: cookieName, Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	}
}

func (s *server) currentUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	s.authMu.Lock()
	defer s.authMu.Unlock()

	session, found := s.sessions[cookie.Value]
	now := time.Now()
	if !found || !session.idleUntil.After(now) || !session.expiresAt.After(now) {
		delete(s.sessions, cookie.Value)
		return "", false
	}
	session.idleUntil = now.Add(sessionIdle)
	s.sessions[cookie.Value] = session
	return session.username, true
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(input.Username))
	now := time.Now()
	s.authMu.Lock()
	if !s.retryAfter.After(now) {
		s.failures = 0
		s.retryAfter = now.Add(time.Minute)
	}
	if s.failures >= 8 {
		s.authMu.Unlock()
		w.Header().Set("Retry-After", "60")
		jsonError(w, http.StatusTooManyRequests, "Espera un minuto para volver a intentar.")
		return
	}
	s.failures++
	passwordVersion := s.passwordVersion
	s.authMu.Unlock()

	var storedUsername string
	var passwordHash []byte
	err := s.db.QueryRowContext(r.Context(), "SELECT username, password_hash FROM users ORDER BY id LIMIT 1").Scan(&storedUsername, &passwordHash)
	if err != nil {
		internalError(w, "consultar usuario", err)
		return
	}
	if len(input.Password) > 72 || bcrypt.CompareHashAndPassword(passwordHash, []byte(input.Password)) != nil || username != storedUsername {
		jsonError(w, http.StatusUnauthorized, "Usuario o contraseña incorrectos.")
		return
	}

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		internalError(w, "crear sesión", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	old, _ := r.Cookie(cookieName)
	s.authMu.Lock()
	if s.passwordVersion != passwordVersion {
		s.authMu.Unlock()
		jsonError(w, http.StatusUnauthorized, "La contraseña cambió. Vuelve a iniciar sesión.")
		return
	}
	for key, session := range s.sessions {
		if !session.expiresAt.After(now) || !session.idleUntil.After(now) {
			delete(s.sessions, key)
		}
	}
	if old != nil {
		delete(s.sessions, old.Value)
	}
	s.sessions[token] = loginSession{username: storedUsername, idleUntil: now.Add(sessionIdle), expiresAt: now.Add(sessionLimit)}
	s.failures = 0
	s.authMu.Unlock()

	http.SetCookie(w, sessionCookie(token, int(sessionLimit.Seconds())))
	s.sendUser(w, storedUsername)
}

func (s *server) sendUser(w http.ResponseWriter, username string) {
	jsonResponse(w, http.StatusOK, struct {
		Username string `json:"username"`
		Demo     bool   `json:"demo"`
	}{username, s.demo})
}

func (s *server) session(w http.ResponseWriter, r *http.Request) {
	s.sendUser(w, actor(r))
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		s.authMu.Lock()
		delete(s.sessions, cookie.Value)
		s.authMu.Unlock()
	}
	http.SetCookie(w, sessionCookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validPassword(input.New) {
		jsonError(w, http.StatusBadRequest, "La contraseña nueva debe tener entre 12 y 72 bytes.")
		return
	}

	username := actor(r)
	var id int64
	var currentHash []byte
	err := s.db.QueryRowContext(r.Context(), "SELECT id, password_hash FROM users WHERE username = ?", username).Scan(&id, &currentHash)
	if errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusUnauthorized, "Inicia sesión para continuar.")
		return
	}
	if err != nil {
		internalError(w, "consultar usuario", err)
		return
	}
	if len(input.Current) > 72 || bcrypt.CompareHashAndPassword(currentHash, []byte(input.Current)) != nil {
		jsonError(w, http.StatusBadRequest, "La contraseña actual es incorrecta.")
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.New), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, "preparar contraseña", err)
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		internalError(w, "cambiar contraseña", err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "UPDATE users SET password_hash = ? WHERE id = ? AND password_hash = ?", passwordHash, id, currentHash)
	if err != nil {
		internalError(w, "cambiar contraseña", err)
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		internalError(w, "cambiar contraseña", err)
		return
	}
	if count == 0 {
		jsonError(w, http.StatusConflict, "La contraseña ya cambió. Vuelve a iniciar sesión.")
		return
	}
	if err := writeCareAudit(r.Context(), tx, username, "password.changed", "user", id, "Contraseña actualizada"); err != nil {
		internalError(w, "registrar cambio de contraseña", err)
		return
	}
	cookie, _ := r.Cookie(cookieName)
	s.authMu.Lock()
	if err := tx.Commit(); err != nil {
		s.authMu.Unlock()
		internalError(w, "guardar contraseña", err)
		return
	}
	s.passwordVersion++
	for key, session := range s.sessions {
		if session.username == username && (cookie == nil || key != cookie.Value) {
			delete(s.sessions, key)
		}
	}
	s.authMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
