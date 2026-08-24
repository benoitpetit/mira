// FingerprintRenderer implementation
package interactors

import (
	"fmt"
	"strings"

	"github.com/benoitpetit/mira/internal/domain/entities"
)

// DefaultFingerprintRenderer implements the FingerprintRenderer interface
type DefaultFingerprintRenderer struct{}

// NewDefaultFingerprintRenderer creates a new renderer
func NewDefaultFingerprintRenderer() *DefaultFingerprintRenderer {
	return &DefaultFingerprintRenderer{}
}

// RenderHeader renders a memory as a header reference
func (r *DefaultFingerprintRenderer) RenderHeader(candidate *entities.Candidate) string {
	return fmt.Sprintf("[%s|%s|%s] → %s",
		candidate.Memory.Type,
		candidate.Verbatim.CreatedAt.Format("2006-01-02"),
		candidate.Verbatim.Wing,
		candidate.Memory.Data.VerbatimRef,
	)
}

// RenderFingerprint renders a memory as a structured fingerprint
func (r *DefaultFingerprintRenderer) RenderFingerprint(candidate *entities.Candidate) string {
	d := candidate.Memory.Data
	dateStr := d.Date
	if len(dateStr) >= 10 {
		dateStr = dateStr[:10]
	} else if dateStr == "" {
		dateStr = "unknown"
	}
	parts := []string{fmt.Sprintf("[%s|%s]", strings.Join(d.Subject, ","), dateStr)}

	if d.Decision != "" {
		parts = append(parts, d.Decision)
	}
	if len(d.Rejected) > 0 {
		parts = append(parts, fmt.Sprintf("(rejected: %s)", strings.Join(d.Rejected, ",")))
	}
	if len(d.Reason) > 0 {
		parts = append(parts, fmt.Sprintf("reason: %s", strings.Join(d.Reason, "; ")))
	}
	if d.Assignee != "" {
		parts = append(parts, fmt.Sprintf("@%s", d.Assignee))
	}
	if d.Deadline != "" {
		parts = append(parts, fmt.Sprintf("due:%s", d.Deadline))
	}

	parts = append(parts, fmt.Sprintf("→ %s", d.VerbatimRef))
	return strings.Join(parts, " | ")
}
