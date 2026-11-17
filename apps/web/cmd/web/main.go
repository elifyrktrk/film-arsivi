// Command web runs the Film Arşivi website.
//
//	go run ./cmd/web
//
// Environment (also read from a .env file next to go.mod when present):
//
//	TMDB_API_KEY  TMDB v3 key or v4 read token (eyJ…). Optional: without it only anime is searched.
//	ADDR          listen address, default 0.0.0.0:8080 so an iPhone on the same Wi-Fi
//	              can open it (PORT alone also works; 127.0.0.1:8080 keeps it on this Mac)
//	DB_PATH       SQLite file, default data/film-arsivi.db
//	LOG_LEVEL     debug | info | warn | error, default info
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
		// IMDb ratings are matched through TMDB's imdb_id, so they are only useful
		// (and only downloaded) when TMDB is on.
		if os.Getenv("IMDB_SYNC") != "off" {
			go keepIMDbFresh(log, st)
		}
	} else {
		log.Warn("TMDB_API_KEY is not set: searching anime only (AniList)")
	}

	srv, err := web.New(search, st, log)
	if err != nil {
		return err
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "0.0.0.0:" + env("PORT", "8080")
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
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	log.Info("listening", "addr", ln.Addr().String(), "db", dbPath, "tmdb", search.TMDBEnabled())
	printURLs(ln.Addr().String())
	go func() { errc <- httpSrv.Serve(ln) }()
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

// printURLs prints where to open the site: on this machine and, when listening on all
// interfaces, from a phone on the same network.
func printURLs(addr string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "\n  Bu Mac'te:      http://localhost:%s\n", port)
	if host == "" || host == "0.0.0.0" || host == "::" {
		for _, ip := range lanIPs() {
			fmt.Fprintf(os.Stderr, "  iPhone'dan:     http://%s:%s   (aynı Wi-Fi)\n", ip, port)
		}
	}
	fmt.Fprintln(os.Stderr)
}

// lanIPs returns this machine's private IPv4 addresses on interfaces that are up.
func lanIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}

// keepIMDbFresh downloads IMDb's ratings dataset at startup when the stored copy is
// older than a day, then checks hourly. A failed download keeps the old data.
func keepIMDbFresh(log *slog.Logger, st *store.SQLite) {
	client := &http.Client{Timeout: 10 * time.Minute}
	check := func() {
		ctx := context.Background()
		updated, err := st.IMDbUpdatedAt(ctx)
		if err != nil {
			log.Warn("imdb: read state", "err", err)
			return
		}
		if time.Since(updated) < 23*time.Hour {
			return
		}
		start := time.Now()
		n, err := sources.SyncIMDbRatings(ctx, client, sources.IMDbRatingsURL, st)
		if err != nil {
			log.Warn("imdb: download failed, keeping previous ratings", "err", err)
			return
		}
		log.Info("imdb: ratings updated", "rows", n, "took", time.Since(start).Round(time.Second))
	}
	check()
	for range time.Tick(time.Hour) {
		check()
	}
}
