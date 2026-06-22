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
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	store.Init(*dataset)
	store.Load(*dataset)

	authCfg := auth.LoadConfig()

	// The _system dataset is the source of truth for users and API keys; it is
	// always loaded. Push its records into the auth config (env still wins),
	// then warn if no admin credential exists anywhere.
	reg := store.DefaultRegistry()
	reg.System() // ensures _system is loaded
	syncSystemCredentials(authCfg)
	bootstrapGuard(authCfg)

	authn := middleware.Authenticator(auth.NewVerifier(authCfg))

	// Preload every dataset referenced by a configured credential, so the
	// first request for each doesn't pay the load cost. The -dataset default
	// is already loaded above; the registry skips datasets already in memory.
	for _, ds := range authCfg.Datasets() {
		if _, ok := reg.Get(ds); ok {
			continue
		}
		reg.Load(ds)
		slog.Info("preloaded credential dataset", "dataset", ds)
	}

	mux := http.NewServeMux()

	rw := []string{"admin", "editor"} // read+write roles

	// Modern routing with method + named wildcards (Go 1.22+).
	// All content/stats access now requires a bearer token; the dataset is
	// resolved from the credential. Only health and login are public.
	mux.HandleFunc("GET /api/health", healthHandler)
	mux.HandleFunc("POST /api/login", loginHandler(authCfg))

	mux.HandleFunc("GET /api/stats", protectHandler(statsHandler, authCfg, authn, rw...))
	mux.HandleFunc("GET /api/metrics", protectHandler(metricsHandler, authCfg, authn, rw...))
	mux.HandleFunc("GET /api/me", protectHandler(meHandler, authCfg, authn, rw...))

	mux.HandleFunc("GET /api/content-types", protectHandler(getContentTypesHandler, authCfg, authn, rw...))
	mux.HandleFunc("POST /api/content-types", protectHandler(createContentTypeHandler, authCfg, authn, "admin"))

	// Content routes with wildcards {contentType} and {id}
	mux.HandleFunc("GET /api/content/{contentType}", protectHandler(listContentHandler, authCfg, authn, rw...))
	mux.HandleFunc("GET /api/content/{contentType}/{id}", protectHandler(getSingleContentHandler, authCfg, authn, rw...))
	mux.HandleFunc("POST /api/content/{contentType}", protectHandler(createContentHandler, authCfg, authn, rw...))
	mux.HandleFunc("PUT /api/content/{contentType}/{id}", protectHandler(updateContentHandler, authCfg, authn, rw...))
	mux.HandleFunc("DELETE /api/content/{contentType}/{id}", protectHandler(deleteContentHandler, authCfg, authn, rw...))

	// Dataset management (admin only)
	mux.HandleFunc("GET /api/datasets", protectHandler(listDatasetsHandler, authCfg, authn, "admin"))
	mux.HandleFunc("GET /api/datasets/{name}", protectHandler(getDatasetHandler, authCfg, authn, "admin"))
	mux.HandleFunc("POST /api/datasets/{name}/load", protectHandler(loadDatasetHandler, authCfg, authn, "admin"))
	mux.HandleFunc("POST /api/datasets/{name}/unload", protectHandler(unloadDatasetHandler, authCfg, authn, "admin"))
	mux.HandleFunc("PATCH /api/datasets/{name}", protectHandler(patchDatasetHandler, authCfg, authn, "admin"))

	// System store management (admin only). The only door into the reserved
	// _system dataset; users + API keys live here.
	mux.HandleFunc("GET /api/system/users", protectHandler(systemUsersHandler(authCfg), authCfg, authn, "admin"))
	mux.HandleFunc("POST /api/system/users", protectHandler(systemUsersHandler(authCfg), authCfg, authn, "admin"))
	mux.HandleFunc("DELETE /api/system/users/{id}", protectHandler(deleteSystemUserHandler(authCfg), authCfg, authn, "admin"))
	mux.HandleFunc("GET /api/system/keys", protectHandler(systemKeysHandler(authCfg), authCfg, authn, "admin"))
	mux.HandleFunc("POST /api/system/keys", protectHandler(systemKeysHandler(authCfg), authCfg, authn, "admin"))
	mux.HandleFunc("DELETE /api/system/keys/{id}", protectHandler(deleteSystemKeyHandler(authCfg), authCfg, authn, "admin"))

	// Uploads (per-dataset). POST resolves the dataset from the credential;
	// GET is public with the dataset in the path so plain <img> tags load.
	mux.HandleFunc("POST /api/uploads", protectHandler(uploadHandler, authCfg, authn, rw...))
	mux.HandleFunc("GET /api/uploads/{dataset}/{name}", serveUploadHandler)

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
		"POST   /api/login",
		"GET    /api/stats                       (auth: admin|editor)",
		"GET    /api/metrics                     (auth: admin|editor)",
		"GET    /api/me                          (auth: admin|editor)",
		"GET    /api/content-types               (auth: admin|editor)",
		"POST   /api/content-types              (auth: admin)",
		"GET    /api/content/{contentType}       (auth: admin|editor)",
		"GET    /api/content/{contentType}/{id}  (auth: admin|editor)",
		"POST   /api/content/{contentType}       (auth: admin|editor)",
		"PUT    /api/content/{contentType}/{id}  (auth: admin|editor)",
		"DELETE /api/content/{contentType}/{id}  (auth: admin|editor)",
		"GET    /api/datasets                    (auth: admin)",
		"GET    /api/datasets/{name}             (auth: admin)",
		"POST   /api/datasets/{name}/load        (auth: admin)",
		"POST   /api/datasets/{name}/unload      (auth: admin)",
		"PATCH  /api/datasets/{name}             (auth: admin)",
		"GET    /api/system/users                (auth: admin)",
		"POST   /api/system/users               (auth: admin)",
		"DELETE /api/system/users/{id}           (auth: admin)",
		"GET    /api/system/keys                 (auth: admin)",
		"POST   /api/system/keys                (auth: admin)",
		"DELETE /api/system/keys/{id}            (auth: admin)",
		"POST   /api/uploads                     (auth: admin|editor)",
		"GET    /api/uploads/{dataset}/{name}",
	}
	for _, r := range routes {
		slog.Info("route registered", "route", r)
	}

	fmt.Println("\nTry with curl:")
	fmt.Println(`  curl -X POST http://localhost:8080/api/content-types -H "Content-Type: application/json" -d '{"name":"post","fields":{"title":"string","body":"text"}}'`)
	fmt.Println(`  curl -X POST http://localhost:8080/api/content/post -H "Content-Type: application/json" -d '{"title":"Hello","body":"World"}'`)
}
