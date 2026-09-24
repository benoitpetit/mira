package agentinstall

import "fmt"

// Policy controls how much of the agent conversation MIRA handles
// automatically. The defaults are intentionally conservative.
type Policy string

const (
	PolicyMinimal  Policy = "minimal"
	PolicyStandard Policy = "standard"
	PolicyComplete Policy = "complete"
)

func ParsePolicy(value string) (Policy, error) {
	policy := Policy(value)
	switch policy {
	case PolicyMinimal, PolicyStandard, PolicyComplete:
		return policy, nil
	default:
		return "", fmt.Errorf("unknown agent policy %q; want minimal, standard, or complete", value)
	}
}

func (p Policy) CaptureUserPrompts() bool { return p == PolicyStandard || p == PolicyComplete }

func (p Policy) CaptureAssistantResponses() bool { return p == PolicyComplete }

func (p Policy) InstructionsEnabled() bool { return true }

func (p Policy) InjectionEnabled() bool { return p == PolicyStandard || p == PolicyComplete }

func (p Policy) InjectionMode() string {
	if p.InjectionEnabled() {
		return "automatic-when-supported"
	}
	return "instruction-guided"
}
