// RelationType value object
package valueobjects

// RelationType represents the type of causal relation
type RelationType string

const (
	RelBecause     RelationType = "BECAUSE"
	RelTriggered   RelationType = "TRIGGERED"
	RelContradicts RelationType = "CONTRADICTS"
	RelUpdates     RelationType = "UPDATES"
	RelResolves    RelationType = "RESOLVES"
)

// IsValid checks if the relation type is valid
func (rt RelationType) IsValid() bool {
	switch rt {
	case RelBecause, RelTriggered, RelContradicts, RelUpdates, RelResolves:
		return true
	}
	return false
}

// String returns the string representation
func (rt RelationType) String() string {
	return string(rt)
}
