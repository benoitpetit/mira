package entities

import "time"

// AccessPolicy represents a bearer token and its associated permissions (wings).
type AccessPolicy struct {
	TokenHash string     `json:"token_hash"` // SHA-256 hash of the bearer token
	Name      string     `json:"name"`       // Descriptive name for the token (e.g. "monitoring-app")
	Wings     []string   `json:"wings"`      // List of allowed wings: read, write, delete, admin
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used"`
}

// HasWing reports whether the policy includes the required wing.
func (p *AccessPolicy) HasWing(required string) bool {
	for _, w := range p.Wings {
		if w == required {
			return true
		}
	}
	return false
}
