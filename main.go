package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/remisb/muxstack/middleware"
	"github.com/remisb/protocms/store"
)

func main() {
	dataset := flag.String("dataset", "default", "name of the dataset to load (stored as data/<name>.json)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	store.Init(*dataset)
	store.Load(*dataset)

	mux := http.NewServeMux()

	// Modern routing with method + named wildcards (Go 1.22+)
	mux.HandleFunc("GET /api/health", healthHandler)
	mux.HandleFunc("GET /api/stats", statsHandler)
	mux.HandleFunc("GET /api/content-types", getContentTypesHandler)
	mux.HandleFunc("POST /api/content-types", createContentTypeHandler)

	// Content routes with wildcards {contentType} and {id}
	mux.HandleFunc("GET /api/content/{contentType}", listContentHandler)
	mux.HandleFunc("GET /api/content/{contentType}/{id}", getSingleContentHandler)
	mux.HandleFunc("POST /api/content/{contentType}", createContentHandler)
	mux.HandleFunc("PUT /api/content/{contentType}/{id}", updateContentHandler)
	mux.HandleFunc("DELETE /api/content/{contentType}/{id}", deleteContentHandler)

	// Uploads
	mux.HandleFunc("POST /api/uploads", uploadHandler)
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
		"POST   /api/content-types",
		"GET    /api/content/{contentType}",
		"GET    /api/content/{contentType}/{id}",
		"POST   /api/content/{contentType}",
		"PUT    /api/content/{contentType}/{id}",
		"DELETE /api/content/{contentType}/{id}",
		"POST   /api/uploads",
		"GET    /api/uploads/{name}",
	}
	for _, r := range routes {
		slog.Info("route registered", "route", r)
	}

	fmt.Println("\nTry with curl:")
	fmt.Println(`  curl -X POST http://localhost:8080/api/content-types -H "Content-Type: application/json" -d '{"name":"post","fields":{"title":"string","body":"text"}}'`)
	fmt.Println(`  curl -X POST http://localhost:8080/api/content/post -H "Content-Type: application/json" -d '{"title":"Hello","body":"World"}'`)
}
