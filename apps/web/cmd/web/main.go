// Command web runs the Film Arşivi website.
//
//	go run ./cmd/web
//
// Environment (also read from a .env file next to go.mod when present):
//
//	TMDB_API_KEY  TMDB v3 key or v4 read token (eyJ…). Optional: without it only anime is searched.
//	ADDR          listen address, default 127.0.0.1:8080 (PORT alone also works)
//	DB_PATH       SQLite file, default data/film-arsivi.db
//	LOG_LEVEL     debug | info | warn | error, default info
package main

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"filmarsivi/web/internal/domain"
	"filmarsivi/web/internal/sources"
	"filmarsivi/web/internal/store"
	"filmarsivi/web/internal/web"
)

func main() {
	loadDotEnv(".env")
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel(os.Getenv("LOG_LEVEL"))}))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	dbPath := env("DB_PATH", filepath.Join("data", "film-arsivi.db"))
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	client := &http.Client{Timeout: 8 * time.Second}
	search := &domain.SearchService{AniList: sources.NewAniList(client)}
	if key := strings.TrimSpace(os.Getenv("TMDB_API_KEY")); key != "" {
		search.TMDB = sources.NewTMDB(client, key)
	} else {
		log.Warn("TMDB_API_KEY is not set: searching anime only (AniList)")
	}

	srv, err := web.New(search, st, log)
	if err != nil {
		return err
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:" + env("PORT", "8080")
	}
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "url", "http://"+addr, "db", dbPath, "tmdb", search.TMDBEnabled())
		errc <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

// loadDotEnv sets KEY=VALUE lines from path for keys not already in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
