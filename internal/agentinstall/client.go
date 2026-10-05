package agentinstall

// IntegrationMode describes how a client receives MIRA recall or capture.
// It is persisted in the manifest so status and doctor report what is actually
// available rather than inferring from the selected policy alone.
type IntegrationMode string

const (
	IntegrationNone        IntegrationMode = "none"
	IntegrationHook        IntegrationMode = "hook"
	IntegrationSkillGuided IntegrationMode = "skill-guided"
)

// Client describes the installation capabilities exposed by one supported
// agent. Client-specific paths and serializers remain in the command adapter.
type Client struct {
	ID               string
	RecallMode       IntegrationMode
	CaptureMode      IntegrationMode
	ProjectScope     bool
	UserScope        bool
	NativeSkill      bool
	SessionStartHook bool
	AssistantHook    bool
}

var clients = map[string]Client{
	"codex":          {ID: "codex", RecallMode: IntegrationHook, CaptureMode: IntegrationHook, ProjectScope: true, UserScope: true, NativeSkill: true, SessionStartHook: true, AssistantHook: true},
	"claude-code":    {ID: "claude-code", RecallMode: IntegrationHook, CaptureMode: IntegrationHook, ProjectScope: true, UserScope: true, NativeSkill: true, SessionStartHook: true, AssistantHook: true},
	"windsurf":       {ID: "windsurf", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationHook, ProjectScope: true, UserScope: true, NativeSkill: true, AssistantHook: true},
	"cursor":         {ID: "cursor", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationNone, ProjectScope: true, UserScope: true, NativeSkill: true},
	"claude-desktop": {ID: "claude-desktop", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationNone, ProjectScope: true, UserScope: true},
	"hermes":         {ID: "hermes", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationNone, ProjectScope: true, UserScope: true, NativeSkill: true},
	"opencode":       {ID: "opencode", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationNone, ProjectScope: true, UserScope: true, NativeSkill: true},
	"pi":             {ID: "pi", RecallMode: IntegrationSkillGuided, CaptureMode: IntegrationNone, ProjectScope: true, UserScope: true, NativeSkill: true},
}

func LookupClient(id string) (Client, bool) {
	client, ok := clients[id]
	return client, ok
}

func (c Client) SupportsScope(scope string) bool {
	return (scope == ScopeProject && c.ProjectScope) || (scope == ScopeUser && c.UserScope)
}

// Modes returns the modes that are active for a selected policy. Minimal keeps
// the installed skill available but does not intercept agent lifecycle events.
func (c Client) Modes(policy Policy) (IntegrationMode, IntegrationMode) {
	if !policy.InjectionEnabled() {
		return IntegrationSkillGuided, IntegrationNone
	}
	recall := c.RecallMode
	capture := c.CaptureMode
	if policy == PolicyComplete && !c.AssistantHook && capture == IntegrationHook {
		capture = IntegrationNone
	}
	return recall, capture
}

func (m IntegrationMode) IsValid() bool {
	return m == IntegrationNone || m == IntegrationHook || m == IntegrationSkillGuided
}
