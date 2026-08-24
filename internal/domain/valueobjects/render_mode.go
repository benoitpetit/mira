// RenderMode value object
package valueobjects

// RenderMode determines how a memory is rendered based on budget
type RenderMode int

const (
	ModeHeader      RenderMode = iota // T2: 2-5 tokens, budget < 100
	ModeFingerprint                   // T1: ~15% tokens, budget 100-999
	ModeCompressed                    // T0c: ~40% tokens, compressed summary
	ModeVerbatim                      // T0: 100% tokens, sufficient budget
)

// String returns the string representation
func (rm RenderMode) String() string {
	switch rm {
	case ModeHeader:
		return "HEADER"
	case ModeFingerprint:
		return "FINGERPRINT"
	case ModeCompressed:
		return "COMPRESSED"
	case ModeVerbatim:
		return "VERBATIM"
	default:
		return "UNKNOWN"
	}
}
