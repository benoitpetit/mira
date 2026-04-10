// MemoryType value object
package valueobjects

// MemoryType represents the type of memory
type MemoryType string

const (
	TypeDecision    MemoryType = "decision"
	TypeFact        MemoryType = "fact"
	TypePreference  MemoryType = "preference"
	TypeSessionNote MemoryType = "session_note"
	TypeDebugLog    MemoryType = "debug_log"
)

// IsValid checks if the memory type is valid
func (mt MemoryType) IsValid() bool {
	switch mt {
	case TypeDecision, TypeFact, TypePreference, TypeSessionNote, TypeDebugLog:
		return true
	}
	return false
}

// DecayRate returns the decay rate per day for this memory type
func (mt MemoryType) DecayRate() float64 {
	switch mt {
	case TypeDecision:
		return 0.001
	case TypeFact:
		return 0.005
	case TypePreference:
		return 0.01
	case TypeSessionNote:
		return 0.1
	case TypeDebugLog:
		return 0.5
	default:
		return 0.1
	}
}

// String returns the string representation
func (mt MemoryType) String() string {
	return string(mt)
}
