package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

type server struct {
	db              *sql.DB
	dataDir         string
	demo            bool
	authMu          sync.Mutex
	sessions        map[string]loginSession
	failures        int
	retryAfter      time.Time
	passwordVersion uint64
}

func main() {
	demo := flag.Bool("demo", false, "Habilita una base de demostración para datos ficticios")
	port := flag.Int("port", 8090, "Puerto en 127.0.0.1")
	data := flag.String("data", "", "Ruta a la carpeta privada de datos")
	flag.Parse()

	if *port < 1 || *port > 65535 {
		log.Fatal("El puerto debe estar entre 1 y 65535")
	}
	if *data == "" {
		*data = "data"
		if *demo {
			*data = "demo-data"
		}
	}

	dataDir, err := filepath.Abs(*data)
	if err != nil {
		log.Fatal(err)
	}
	db, err := openDatabase(dataDir, *demo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	s := &server{db: db, dataDir: dataDir, demo: *demo, sessions: make(map[string]loginSession)}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	httpServer := &http.Server{
		Addr: address, Handler: s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       25 * time.Second, WriteTimeout: 35 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("No se pudo iniciar BIODENTIS en %s: %v", address, err)
	}
	log.Printf("BIODENTIS disponible en http://%s", address)
	if *demo {
		log.Println("Modo demostración. Utiliza únicamente datos ficticios.")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print(err)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("POST /api/password", s.changePassword)
	mux.HandleFunc("GET /api/patients", s.listPatients)
	mux.HandleFunc("POST /api/patients", s.createPatient)
	mux.HandleFunc("GET /api/patients/{id}", s.getPatient)
	mux.HandleFunc("PUT /api/patients/{id}", s.updatePatient)
	mux.HandleFunc("GET /api/appointments", s.listAppointments)
	mux.HandleFunc("POST /api/appointments", s.createAppointment)
	mux.HandleFunc("PUT /api/appointments/{id}", s.updateAppointment)
	s.registerCare(mux)

	web, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	static := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if hostname, _, err := net.SplitHostPort(host); err == nil {
			host = hostname
		}
		if host != "127.0.0.1" && !strings.EqualFold(host, "localhost") {
			jsonError(w, http.StatusForbidden, "Abre BIODENTIS desde la dirección local.")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.Header().Set("Cache-Control", "no-store")
			jsonResponse(w, http.StatusOK, map[string]any{"app": "biodentis", "demo": s.demo})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-cache")
			static.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			jsonError(w, http.StatusForbidden, "La solicitud debe originarse en BIODENTIS.")
			return
		}
		if r.URL.Path != "/api/login" {
			user, ok := s.currentUser(r)
			if !ok {
				jsonError(w, http.StatusUnauthorized, "Inicia sesión para continuar.")
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), actorKey{}, user))
		}
		if _, pattern := mux.Handler(r); pattern == "" {
			jsonError(w, http.StatusNotFound, "Operación no disponible.")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		return err == nil && parsed.Scheme == "http" && parsed.User == nil && strings.EqualFold(parsed.Host, r.Host) && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
	}
	return true
}

type actorKey struct{}

func actor(r *http.Request) string {
	username, _ := r.Context().Value(actorKey{}).(string)
	return username
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Error al enviar respuesta JSON: %v", err)
	}
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		jsonError(w, http.StatusUnsupportedMediaType, "Envía la solicitud como JSON.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dest); err != nil {
		jsonError(w, http.StatusBadRequest, "Revisa los datos ingresados.")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		jsonError(w, http.StatusBadRequest, "Envía un solo objeto JSON.")
		return false
	}
	return true
}

func internalError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	jsonError(w, http.StatusInternalServerError, "Ocurrió un error al guardar o consultar los datos.")
}

func requestedID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}
