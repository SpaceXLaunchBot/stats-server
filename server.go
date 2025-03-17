package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"
)

const generateStatsResponseTimeout = 5 * time.Second

type server struct {
	db            *pgxpool.Pool
	dbCacheTime   time.Duration
	lastRespBytes []byte
	lastUpdated   time.Time
	lastFieldsMu  sync.RWMutex
}

func (s *server) generateStatsRespJson(ctx context.Context) ([]byte, error) {
	var countRecords []countRecord
	// Get the highest of each count for each day
	err := pgxscan.Select(ctx, s.db, &countRecords, `
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
	err = pgxscan.Select(ctx, s.db, &actionCounts, `
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

	s.lastFieldsMu.RLock()
	if time.Since(s.lastUpdated) < s.dbCacheTime {
		// If we're within the cache time, return cached response
		// Copy so we don't have to hold the lock for w.Write
		responseJson = make([]byte, len(s.lastRespBytes))
		copy(responseJson, s.lastRespBytes)
	}
	s.lastFieldsMu.RUnlock()

	// This could be an else block, but it helps avoid confusing mutex usage
	if len(responseJson) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), generateStatsResponseTimeout)
		defer cancel()

		responseJson, err = s.generateStatsRespJson(ctx)
		if err != nil {
			log.Println("Failed to generate response:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		s.lastFieldsMu.Lock()
		s.lastRespBytes = responseJson
		s.lastUpdated = time.Now()
		s.lastFieldsMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(responseJson)))
	_, _ = w.Write(responseJson)
}
