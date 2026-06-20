package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/remisb/muxstack/middleware"
	"github.com/remisb/protocms/internal/auth"
	"github.com/remisb/protocms/store"
)

func main() {
	dataset := flag.String("dataset", "default", "name of the dataset to load (stored as data/<name>/ or legacy data/<name>.json)")
	migrate := flag.Bool("migrate", false, "convert the -dataset dataset from the legacy flat file to the new folder format, then exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if *migrate {
		res, err := store.MigrateDataset(*dataset)
		if err != nil {
			slog.Error("migration failed", "dataset", *dataset, "err", err)
			os.Exit(1)
		}
		slog.Info("migration complete",
			"dataset", res.Name,
			"from", res.OldFile,
			"to", res.NewDir,
			"types", res.Types,
			"items", res.Items)
		fmt.Printf("migrated %q: %s -> %s/ (%d types, %d items); old file kept\n",
			res.Name, res.OldFile, res.NewDir, res.Types, res.Items)
		return
	}

	store.Init(*dataset)
	store.Load(*dataset)

	authCfg := auth.LoadConfig()
	authn := middleware.Authenticator(auth.NewVerifier(authCfg))

	mux := http.NewServeMux()

	// Modern routing with method + named wildcards (Go 1.22+)
	// Reads are public; writes require a bearer token (API key or JWT).
	mux.HandleFunc("GET /api/health", healthHandler)
	mux.HandleFunc("GET /api/stats", statsHandler)
	mux.HandleFunc("GET /api/content-types", getContentTypesHandler)
	mux.HandleFunc("POST /api/content-types", protectHandler(createContentTypeHandler, authn, "admin"))

	// Auth
	mux.HandleFunc("POST /api/login", loginHandler(authCfg))
	mux.HandleFunc("GET /api/me", protectHandler(meHandler, authn, "admin", "editor"))

	// Content routes with wildcards {contentType} and {id}
	mux.HandleFunc("GET /api/content/{contentType}", listContentHandler)
	mux.HandleFunc("GET /api/content/{contentType}/{id}", getSingleContentHandler)
	mux.HandleFunc("POST /api/content/{contentType}", protectHandler(createContentHandler, authn, "admin", "editor"))
	mux.HandleFunc("PUT /api/content/{contentType}/{id}", protectHandler(updateContentHandler, authn, "admin", "editor"))
	mux.HandleFunc("DELETE /api/content/{contentType}/{id}", protectHandler(deleteContentHandler, authn, "admin", "editor"))

	// Uploads
	mux.HandleFunc("POST /api/uploads", protectHandler(uploadHandler, authn, "admin", "editor"))
	mux.HandleFunc("GET /api/uploads/{name}", serveUploadHandler)

	handler := middleware.Chain(
		mux,
		middleware.CORS(middleware.DefaultCORSConfig()),
		middleware.Logger(logger),
		middleware.Recoverer(logger),
		//middleware.RateLimiter(middleware.RateLimitConfig{
		//	RequestsPerInterval: 2,
		//	Interval:            time.Second,
		//	KeyFunc: func(r *http.Request) string {
		//		if key := r.Header.Get("X-API-Key"); key != "" {
		//			return key
		//		}
		//		return remoteIP(r) // fallback
		//	},
		//}),
		//middleware.Timeout(5*time.Second), // ← wraps all handlers
	)

	fmt.Println("🚀 POC In-Memory Headless CMS (Go 1.26 stdlib + modern ServeMux) on :8080")
	fmt.Println("In-memory storage — perfect for Jamstack / frontend testing")
	printRoutes()

	if err := http.ListenAndServe(":8080", handler); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func printRoutes() {
	routes := []string{
		"GET    /api/health",
		"GET    /api/stats",
		"GET    /api/content-types",
		"POST   /api/content-types              (auth: admin)",
		"POST   /api/login",
		"GET    /api/me                          (auth: admin|editor)",
		"GET    /api/content/{contentType}",
		"GET    /api/content/{contentType}/{id}",
		"POST   /api/content/{contentType}       (auth: admin|editor)",
		"PUT    /api/content/{contentType}/{id}  (auth: admin|editor)",
		"DELETE /api/content/{contentType}/{id}  (auth: admin|editor)",
		"POST   /api/uploads                     (auth: admin|editor)",
		"GET    /api/uploads/{name}",
	}
	for _, r := range routes {
		slog.Info("route registered", "route", r)
	}

	fmt.Println("\nTry with curl:")
	fmt.Println(`  curl -X POST http://localhost:8080/api/content-types -H "Content-Type: application/json" -d '{"name":"post","fields":{"title":"string","body":"text"}}'`)
	fmt.Println(`  curl -X POST http://localhost:8080/api/content/post -H "Content-Type: application/json" -d '{"title":"Hello","body":"World"}'`)
}
