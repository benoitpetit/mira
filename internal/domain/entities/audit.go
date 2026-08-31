package entities

import "time"

// AuditLog represents a record of a system operation for traceability.
type AuditLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`    // e.g., "store", "recall", "delete", "consolidate"
	Actor     string    `json:"actor"`     // e.g., token hash or wing name
	Resource  string    `json:"resource"`  // e.g., memory ID, room name, wing name
	Status    int       `json:"status"`    // HTTP status code or success/failure indicator
	Metadata  string    `json:"metadata"`  // JSON blob for extra context (e.g. query, duration)
}
