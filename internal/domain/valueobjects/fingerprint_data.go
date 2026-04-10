// FingerprintData value object
package valueobjects

// FingerprintData contains the structured extracted information
type FingerprintData struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Date        string         `json:"date"`
	Entities    []string       `json:"entities"`
	Subject     []string       `json:"subject"`
	Decision    string         `json:"decision,omitempty"`
	Rejected    []string       `json:"rejected,omitempty"`
	Reason      []string       `json:"reason,omitempty"`
	ValidatedBy string         `json:"validated_by,omitempty"`
	Assignee    string         `json:"assignee,omitempty"`
	Deadline    string         `json:"deadline,omitempty"`
	VerbatimRef string         `json:"verbatim_ref"`
	Custom      map[string]any `json:"custom,omitempty"`
}
