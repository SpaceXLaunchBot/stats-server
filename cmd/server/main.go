package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SpaceXLaunchBot/stats-server/internal/config"
)

// Allow 1 request every n seconds to trigger a DB read / calculation.
const globalRateLimit = 10 * time.Second

type countRecord struct {
	GuildCount      int    `db:"guild_count" json:"g"`
	SubscribedCount int    `db:"subscribed_count" json:"s"`
	Date            string `db:"date" json:"d"`
}

type actionCount struct {
	Action string `db:"action_formatted" json:"a"`
	Count  int    `db:"count" json:"c"`
}

type statsResponse struct {
	Counts       []countRecord `json:"counts"`
	ActionCounts []actionCount `json:"action_counts"`
}

type server struct {
	dbPool        *pgxpool.Pool
	lastRespBytes []byte
	lastUpdated   time.Time
	// For r/w to last* fields.
	lastMu sync.Mutex
}

func (s *server) generateStatsRespJson() ([]byte, error) {
	ctx := context.Background()

	var countRecords []countRecord
	// Query constructed with help from ChatGPT!
	// Get the highest of each count for each day.
	err := pgxscan.Select(ctx, s.dbPool, &countRecords, `
		SELECT
			guild_count,
			subscribed_count,
			to_char("time", 'YYYY-MM-DD') AS "date"
		FROM (
			SELECT
				MAX(guild_count) AS guild_count,
				MAX(subscribed_count) AS subscribed_count,
				date_trunc('day', "time") AS "time"
			FROM counts
			GROUP BY date_trunc('day', "time")
			ORDER BY "time"
		) AS subquery;`,
	)
	if err != nil {
		return nil, err
	}

	var actionCounts []actionCount
	err = pgxscan.Select(ctx, s.dbPool, &actionCounts, `
		SELECT
			replace(replace(replace(action, 'command_', ''), '_cmd', ''), '_', '') AS "action_formatted",
			count(action) AS "count"
		FROM metrics
		WHERE action LIKE 'command_%'
		GROUP BY "action_formatted";`,
	)
	if err != nil {
		return nil, err
	}

	response := &statsResponse{
		Counts:       countRecords,
		ActionCounts: actionCounts,
	}

	responseJson, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return responseJson, nil
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var responseJson []byte
	var err error

	s.lastMu.Lock()

	if time.Since(s.lastUpdated) < globalRateLimit {
		// If rate limit is hit, return cached response.
		// Copying means we don't have to hold the lock for w.Write, which might be slow or hang.
		responseJson = make([]byte, len(s.lastRespBytes))
		copy(responseJson, s.lastRespBytes)
	} else {
		responseJson, err = s.generateStatsRespJson()
		if err != nil {
			s.lastMu.Unlock()
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		s.lastRespBytes = responseJson
		s.lastUpdated = time.Now()
	}

	s.lastMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(responseJson)))
	w.Write(responseJson)
}

func main() {
	// TODO: Properly use contexts everywhere.
	ctx := context.Background()

	c, err := config.Get()
	if err != nil {
		log.Fatalf("Failed to get config: %s", err)
	}
	log.Println("Config loaded")
	log.Printf("DbHost: %s", c.DbHost)
	log.Printf("DbPort: %d", c.DbPort)
	log.Printf("DbUser: %s", c.DbUser)
	log.Printf("DbName: %s", c.DbName)

	db, err := pgxpool.New(ctx, fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s",
		c.DbUser, c.DbPass, c.DbHost, c.DbPort, c.DbName,
	))
	if err != nil {
		log.Fatalf("Failed to pool db: %s", err)
	}
	log.Println("Created DB pool")

	err = db.Ping(ctx)
	if err != nil {
		log.Fatalf("Failed to ping db: %s", err)
	}
	log.Println("Confirmed DB connection")

	s := server{
		dbPool:        db,
		lastRespBytes: []byte("{}"),
		// Set an initial value for LastUpdated to a time in the past.
		lastUpdated: time.Now().Add(-time.Minute),
		lastMu:      sync.Mutex{},
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(cors.Default().Handler)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(middleware.Heartbeat("/health"))

	r.Get("/", s.handleRoot)

	log.Println("Server listening on port 8080")
	http.ListenAndServe(":8080", r)
}
