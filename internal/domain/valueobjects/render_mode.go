// RenderMode value object
package valueobjects

// RenderMode determines how a memory is rendered based on budget
type RenderMode int

const (
	ModeHeader      RenderMode = iota // T2: 2-5 tokens, budget < 100
	ModeFingerprint                   // T1: ~15% tokens, budget < 1000
	ModeVerbatim                      // T0: 100% tokens, sufficient budget
)

// String returns the string representation
func (rm RenderMode) String() string {
	switch rm {
	case ModeHeader:
		return "HEADER"
	case ModeFingerprint:
		return "FINGERPRINT"
	case ModeVerbatim:
		return "VERBATIM"
	default:
		return "UNKNOWN"
	}
}
