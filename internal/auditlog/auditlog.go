package auditlog

// Event is a single API request audit row.
type Event struct {
	RequestID  string `json:"request_id"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	ClientIP   string `json:"client_ip"`
}
