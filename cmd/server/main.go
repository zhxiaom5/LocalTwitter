package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"localtwitter/internal/server"
	"localtwitter/web"
)

func main() {
	addr := getenv("LOCALTWITTER_ADDR", ":8080")
	configPath := getenv("LOCALTWITTER_CONFIG", "localtwitter.config.json")
	defaultDBDir := getenv("LOCALTWITTER_DB_DIR", ".")
	webDir := os.Getenv("LOCALTWITTER_WEB_DIR")
	if os.Getenv("LOCALTWITTER_ACCESS_LOG") == "" {
		_ = os.Setenv("LOCALTWITTER_ACCESS_LOG", "access.log")
	}

	webFS, err := web.Dist()
	if err != nil {
		log.Fatal(err)
	}
	app, err := server.NewConfiguredAppWithAssets(configPath, filepath.Clean(defaultDBDir), server.WebAssets{Dir: webDir, FS: webFS})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	log.Printf("LocalTwitter listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, app.Routes()); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
