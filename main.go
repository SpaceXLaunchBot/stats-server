package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// If the cached response is older than this, allow a given request to trigger a DB read
const databaseCacheTime = 5 * time.Second

func main() {
	c := loadConfig()
	log.Printf("Config loaded: %s\n", c.CensoredConnectionString())

	pgSetupCtx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	db, err := pgxpool.New(pgSetupCtx, c.ConnectionString())
	if err != nil {
		log.Fatalf("Failed to connect to db: %s", err)
	}
	defer db.Close()
	log.Println("Created DB connection")

	err = db.Ping(pgSetupCtx)
	if err != nil {
		log.Fatalf("Failed to ping db: %s", err)
	}
	log.Println("Confirmed DB connection")

	s := server{
		db:            db,
		dbCacheTime:   databaseCacheTime,
		lastRespBytes: []byte("{}"),
		// Set an initial value for LastUpdated to a time in the past
		lastUpdated:  time.Now().Add(-time.Hour),
		lastFieldsMu: sync.RWMutex{},
	}

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(cors.Default().Handler)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.GetHead)
	r.Use(middleware.Heartbeat("/health"))
	r.Get("/", s.handleRoot)

	log.Println("Listening on :8080")
	if err = http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Failed to start server: %s", err)
	}
}
