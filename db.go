package main

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
