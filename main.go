package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if err := os.MkdirAll("data", 0755); err != nil {
		panic(err)
	}
	storeLoad()

	mux := http.NewServeMux()

	// Modern routing with method + named wildcards (Go 1.22+)
	mux.HandleFunc("GET /api/health", healthHandler)
	mux.HandleFunc("GET /api/content-types", getContentTypesHandler)
	mux.HandleFunc("POST /api/content-types", createContentTypeHandler)

	// Content routes with wildcards {contentType} and {id}
	mux.HandleFunc("GET /api/content/{contentType}", listContentHandler)
	mux.HandleFunc("GET /api/content/{contentType}/{id}", getSingleContentHandler)
	mux.HandleFunc("POST /api/content/{contentType}", createContentHandler)
	mux.HandleFunc("PUT /api/content/{contentType}/{id}", updateContentHandler)
	mux.HandleFunc("DELETE /api/content/{contentType}/{id}", deleteContentHandler)

	fmt.Println("🚀 POC In-Memory Headless CMS (Go 1.26 stdlib + modern ServeMux) on :8080")
	fmt.Println("In-memory storage — perfect for Jamstack / frontend testing")
	printRoutes()

	if err := http.ListenAndServe(":8080", mux); err != nil {
		panic(err)
	}
}

func printRoutes() {
	fmt.Println("\nAvailable Endpoints (with named wildcards):")
	fmt.Println("  GET    /api/health")
	fmt.Println("  GET    /api/content-types")
	fmt.Println("  POST   /api/content-types")
	fmt.Println("  GET    /api/content/{contentType}")
	fmt.Println("  GET    /api/content/{contentType}/{id}")
	fmt.Println("  POST   /api/content/{contentType}")
	fmt.Println("  PUT    /api/content/{contentType}/{id}")
	fmt.Println("  DELETE /api/content/{contentType}/{id}")
	fmt.Println("\nTry with curl:")
	fmt.Println(`  curl -X POST http://localhost:8080/api/content-types -H "Content-Type: application/json" -d '{"name":"post","fields":{"title":"string","body":"text"}}'`)
	fmt.Println(`  curl -X POST http://localhost:8080/api/content/post -H "Content-Type: application/json" -d '{"title":"Hello","body":"World"}'`)
}
