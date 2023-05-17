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

func writeJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func (s *server) getStats() (*statsResponse, error) {
	ctx := context.Background()

	var countRecords []countRecord
	err := pgxscan.Select(ctx, s.dbPool, &countRecords, `
		SELECT
			guild_count,
			subscribed_count,
			to_char("time", 'YYYY-MM-DD HH24:00:00') AS "date"
		FROM counts;`,
	)
	if err != nil {
		return nil, err
	}

	var actionCounts []actionCount
	err = pgxscan.Select(ctx, s.dbPool, &actionCounts, `
		SELECT
			replace(replace(replace(action, 'command_', ''), '_cmd', ''), '_', '') as "action_formatted",
			count(action) as "count"
		FROM metrics
		WHERE action like 'command_%'
		GROUP BY "action_formatted";`,
	)
	if err != nil {
		return nil, err
	}

	return &statsResponse{
		Counts:       countRecords,
		ActionCounts: actionCounts,
	}, nil
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// If rate limit is hit, return cached response.
	s.lastMu.Lock()
	if time.Since(s.lastUpdated) < globalRateLimit {
		// Copying means we don't have to hold the lock for w.Write, which might be slow or hang.
		responseJSON := make([]byte, len(s.lastRespBytes))
		copy(responseJSON, s.lastRespBytes)
		s.lastMu.Unlock()

		writeJSON(w, responseJSON)
		return
	}
	s.lastMu.Unlock()

	stats, err := s.getStats()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	responseJson, err := json.Marshal(stats)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.lastMu.Lock()
	s.lastRespBytes = responseJson
	s.lastUpdated = time.Now()
	s.lastMu.Unlock()

	writeJSON(w, responseJson)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	c, err := config.Get()
	if err != nil {
		log.Fatalf("Failed to get config: %s", err)
	}

	log.Println("Config loaded")
	log.Printf("DbHost: %s", c.DbHost)
	log.Printf("DbPort: %d", c.DbPort)
	log.Printf("DbUser: %s", c.DbUser)
	log.Printf("DbName: %s", c.DbName)

	dbConnStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s",
		c.DbUser, c.DbPass, c.DbHost, c.DbPort, c.DbName,
	)

	db, err := pgxpool.New(context.Background(), dbConnStr)
	if err != nil {
		log.Fatalf("Failed to pool db: %s", err)
	}

	s := server{
		dbPool:        db,
		lastRespBytes: []byte("{}"),
		// Set an initial value for LastUpdated to a time in the past
		lastUpdated: time.Now().Add(-time.Minute),
		lastMu:      sync.Mutex{},
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", s.handleRoot)
	r.Get("/debug/health", health)

	log.Println("Server listening on port 8080")
	http.ListenAndServe(":8080", r)
}
