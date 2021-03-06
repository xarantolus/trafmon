package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v43/github"
	_ "github.com/lib/pq"
	"github.com/xarantolus/trafmon/app/config"
	"github.com/xarantolus/trafmon/app/store"
	"github.com/xarantolus/trafmon/app/web"
	"golang.org/x/oauth2"
)

//go:embed frontend
var frontendFS embed.FS

func main() {
	// Set one up @ https://github.com/settings/tokens/new
	cfg, err := config.FromEnvironment()
	if err != nil {
		log.Fatalf("[Startup] Getting config from environment: %s\n", err.Error())
	}

	ctx := context.Background()
	token := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: cfg.GitHubToken},
	))
	ghClient := github.NewClient(token)

	// store.FetchHistoricalStats(ctx, ghClient, "xarantolus", "filtrite")

	// Connect to database
	psqlInfo := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", cfg.DB.User, cfg.DB.Password, cfg.DB.Host, cfg.DB.Port, cfg.DB.DBName)
	database, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatalf("[Startup] Creating database connection: %s", err)
	}

	var tries = 1
	for {
		err = database.Ping()
		if err == nil || tries > 30 {
			break
		}
		log.Printf("[Startup] Waiting for database to come online: %s", err.Error())
		time.Sleep(time.Second * time.Duration(tries))
		tries++
	}
	if err != nil {
		log.Fatalf("[Startup] Database did not come online: %s", err.Error())
	}

	var manager = &store.Manager{
		Database: database,
		GitHub:   ghClient,
	}

	if cfg.DisableBackgroundChecks {
		log.Printf("[Startup] Not running background tasks")
	} else {
		err = manager.StartBackgroundTasks()
		if err != nil {
			log.Fatalf("[Startup] Starting background tasks: %s\n", err.Error())
		}
	}

	defer panic("web server should never stop, but did")
	err = (&web.Server{Manager: manager, Frontend: frontendFS}).Run(cfg)
	if err != nil {
		log.Fatalf("[Startup] Running web server: %s\n", err.Error())
	}
}
