package main

import (
	"log"
	"os"

	"github.com/Wingrammer/kanban-backend/kanban"
	"github.com/Wingrammer/kanban-backend/supabase"
	"github.com/joho/godotenv"
)

type Config struct {
	SupabaseURL        string
	SupabasePublishKey string
	Port               string
}

func main() {

	// 1. Load .env file safely (ignore error in production environments like Render/Heroku)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, fetching from system environment variables")
	}

	// 2. Map environment variables into a strong config struct
	cfg := Config{
		SupabaseURL:        os.Getenv("SUPABASE_URL"),
		SupabasePublishKey: os.Getenv("SUPABASE_PUBLISHABLE_KEY"),
		Port:               os.Getenv("PORT"),
	}

	// 3. Fail fast if essential secrets are missing
	if cfg.SupabaseURL == "" || cfg.SupabasePublishKey == "" {
		log.Fatal("Critical Error: SUPABASE_URL and SUPABASE_PUBLISHABLE_KEY are required")
	}

	if cfg.Port == "" {
		cfg.Port = "8080" // Default fallback
	}

	supabaseClient, err := supabase.NewClient(cfg.SupabaseURL, cfg.SupabasePublishKey, nil)
	if err != nil {
		log.Fatal(err)
	}
	repository := kanban.NewSupabaseRepository(supabaseClient)
	service := kanban.NewTaskService(repository)
	if err := kanban.NewRouter(service).Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
