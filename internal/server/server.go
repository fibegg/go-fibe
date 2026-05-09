package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/fibegg/go-fibe/graph"
	"github.com/fibegg/go-fibe/graph/generated"
	"github.com/fibegg/go-fibe/internal/ability"
	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/appauth"
	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/event"
	"github.com/fibegg/go-fibe/internal/security"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Serve(ctx context.Context, cfg config.Config) error {
	rt, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Close()
	go event.ForwardRedis(ctx, rt.Redis, rt.Events)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(corsMiddleware(cfg))
	router.Use(security.HostGuard(rt))
	router.Use(security.RateLimit(rt))
	router.Use(security.SecurityHeaders(cfg))

	router.Get("/up", up)
	router.Get("/readyz", readyz(rt))
	router.Get("/metrics", metrics)
	router.Get("/api/events", events(rt))
	router.Post("/auth/login", appauth.Login(rt))
	router.Post("/auth/logout", appauth.Logout(rt))
	router.Get("/auth/session", appauth.Session(rt))
	router.Handle("/graphql", graphqlHandler(rt))
	if cfg.IsDevelopment() {
		router.Handle("/graphiql", playground.Handler("GraphQL", "/graphql"))
	}
	router.NotFound(apiNotFound)

	server := &http.Server{
		Addr:              cfg.BindAddr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("serving web", "addr", cfg.BindAddr())
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func graphqlHandler(rt *app.App) http.Handler {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: &graph.Resolver{Runtime: rt}}))
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.GET{})
	srv.Use(extension.Introspection{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, ok := appauth.CurrentUser(rt, r); ok {
			r = r.WithContext(ability.WithUser(r.Context(), user))
		}
		srv.ServeHTTP(w, r)
	})
}

func up(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func readyz(rt *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbOK := rt.Store.Pool().Ping(r.Context()) == nil
		redisOK := rt.Redis.Ping(r.Context()).Err() == nil
		status := http.StatusOK
		if !dbOK || !redisOK {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]bool{"database": dbOK, "redis": redisOK})
	}
}

func metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte("# HELP uptime_console_info Starter app info\n# TYPE uptime_console_info gauge\nuptime_console_info 1\n"))
}

func events(rt *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		id, ch := rt.Events.Subscribe()
		defer rt.Events.Unsubscribe(id)
		for {
			select {
			case <-r.Context().Done():
				return
			case payload := <-ch:
				_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
				flusher.Flush()
			case <-time.After(25 * time.Second):
				_, _ = w.Write([]byte(": keepalive\n\n"))
				flusher.Flush()
			}
		}
	}
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func corsMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if cfg.AllowsAllOrigins() {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if originAllowed(origin, cfg.CORSAllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == origin {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
