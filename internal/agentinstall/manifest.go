package agentinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ScopeProject = "project"
	ScopeUser    = "user"

	defaultRecallBudget = 800
	defaultMinChars     = 20
)

var wingPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// Manifest is the local contract between an installed client adapter and the
// MIRA event bridge. It is deliberately separate from config.yaml so an
// adapter can be removed without changing the memory database configuration.
type Manifest struct {
	Client string `yaml:"-"`
	Scope  string `yaml:"-"`
	Wing   string `yaml:"-"`
	Policy Policy `yaml:"-"`

	Capture CaptureConfig `yaml:"capture"`
	Recall  RecallConfig  `yaml:"recall"`
	Soul    SoulConfig    `yaml:"soul"`
}

type CaptureConfig struct {
	UserPrompts         bool `yaml:"user_prompts"`
	AssistantResponses  bool `yaml:"assistant_responses"`
	MinChars            int  `yaml:"min_chars"`
	RedactSecrets       bool `yaml:"redact_secrets"`
	DeduplicateSessions bool `yaml:"deduplicate_sessions"`
}

type RecallConfig struct {
	Enabled      bool `yaml:"enabled"`
	BeforeTask   bool `yaml:"before_task"`
	SessionStart bool `yaml:"session_start"`
	BudgetTokens int  `yaml:"budget_tokens"`
}

type SoulConfig struct {
	Enabled          bool `yaml:"enabled"`
	ObserveAssistant bool `yaml:"observe_assistant"`
	AutoApplyTraits  bool `yaml:"auto_apply_traits"`
}

type manifestFile struct {
	Agent   manifestAgent `yaml:"agent"`
	Capture CaptureConfig `yaml:"capture"`
	Recall  RecallConfig  `yaml:"recall"`
	Soul    SoulConfig    `yaml:"soul"`
}

type manifestAgent struct {
	Client string `yaml:"client"`
	Scope  string `yaml:"scope"`
	Wing   string `yaml:"wing"`
	Policy Policy `yaml:"policy"`
}

type rawManifestFile struct {
	Agent struct {
		Client string `yaml:"client"`
		Scope  string `yaml:"scope"`
		Wing   string `yaml:"wing"`
		Policy string `yaml:"policy"`
	} `yaml:"agent"`
	Capture struct {
		UserPrompts         *bool `yaml:"user_prompts"`
		AssistantResponses  *bool `yaml:"assistant_responses"`
		MinChars            *int  `yaml:"min_chars"`
		RedactSecrets       *bool `yaml:"redact_secrets"`
		DeduplicateSessions *bool `yaml:"deduplicate_sessions"`
	} `yaml:"capture"`
	Recall struct {
		Enabled      *bool `yaml:"enabled"`
		BeforeTask   *bool `yaml:"before_task"`
		SessionStart *bool `yaml:"session_start"`
		BudgetTokens *int  `yaml:"budget_tokens"`
	} `yaml:"recall"`
	Soul struct {
		Enabled          *bool `yaml:"enabled"`
		ObserveAssistant *bool `yaml:"observe_assistant"`
		AutoApplyTraits  *bool `yaml:"auto_apply_traits"`
	} `yaml:"soul"`
}

func DefaultManifest(client, projectRoot string) Manifest {
	wing := "project"
	if resolved, err := ResolveProjectWing(projectRoot); err == nil {
		wing = resolved
	}
	return Manifest{
		Client: client,
		Scope:  ScopeProject,
		Wing:   wing,
		Policy: PolicyStandard,
		Capture: CaptureConfig{
			UserPrompts:         true,
			AssistantResponses:  false,
			MinChars:            defaultMinChars,
			RedactSecrets:       true,
			DeduplicateSessions: true,
		},
		Recall: RecallConfig{Enabled: true, BeforeTask: true, SessionStart: true, BudgetTokens: defaultRecallBudget},
		Soul:   SoulConfig{Enabled: true, ObserveAssistant: false, AutoApplyTraits: false},
	}
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	var raw rawManifestFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("decode agent manifest: %w", err)
	}
	manifest := DefaultManifest(raw.Agent.Client, filepath.Dir(filepath.Dir(path)))
	if raw.Agent.Scope != "" {
		manifest.Scope = raw.Agent.Scope
	}
	if raw.Agent.Wing != "" {
		manifest.Wing = raw.Agent.Wing
	}
	if raw.Agent.Policy != "" {
		manifest.Policy = Policy(raw.Agent.Policy)
	}
	if raw.Capture.UserPrompts != nil {
		manifest.Capture.UserPrompts = *raw.Capture.UserPrompts
	}
	if raw.Capture.AssistantResponses != nil {
		manifest.Capture.AssistantResponses = *raw.Capture.AssistantResponses
	}
	if raw.Capture.MinChars != nil {
		manifest.Capture.MinChars = *raw.Capture.MinChars
	}
	if raw.Capture.RedactSecrets != nil {
		manifest.Capture.RedactSecrets = *raw.Capture.RedactSecrets
	}
	if raw.Capture.DeduplicateSessions != nil {
		manifest.Capture.DeduplicateSessions = *raw.Capture.DeduplicateSessions
	}
	if raw.Recall.Enabled != nil {
		manifest.Recall.Enabled = *raw.Recall.Enabled
	}
	if raw.Recall.BeforeTask != nil {
		manifest.Recall.BeforeTask = *raw.Recall.BeforeTask
	}
	if raw.Recall.SessionStart != nil {
		manifest.Recall.SessionStart = *raw.Recall.SessionStart
	}
	if raw.Recall.BudgetTokens != nil {
		manifest.Recall.BudgetTokens = *raw.Recall.BudgetTokens
	}
	if raw.Soul.Enabled != nil {
		manifest.Soul.Enabled = *raw.Soul.Enabled
	}
	if raw.Soul.ObserveAssistant != nil {
		manifest.Soul.ObserveAssistant = *raw.Soul.ObserveAssistant
	}
	if raw.Soul.AutoApplyTraits != nil {
		manifest.Soul.AutoApplyTraits = *raw.Soul.AutoApplyTraits
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func SaveManifest(path string, manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(manifestFile{
		Agent:   manifestAgent{Client: manifest.Client, Scope: manifest.Scope, Wing: manifest.Wing, Policy: manifest.Policy},
		Capture: manifest.Capture,
		Recall:  manifest.Recall,
		Soul:    manifest.Soul,
	})
	if err != nil {
		return fmt.Errorf("encode agent manifest: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create agent manifest directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agent.yaml.*.tmp")
	if err != nil {
		return fmt.Errorf("create agent manifest temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set agent manifest permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write agent manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent manifest: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install agent manifest: %w", err)
	}
	return nil
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Client) == "" {
		return errors.New("agent manifest client is required")
	}
	if m.Scope != ScopeProject && m.Scope != ScopeUser {
		return fmt.Errorf("agent manifest scope %q is invalid", m.Scope)
	}
	if _, err := ParsePolicy(string(m.Policy)); err != nil {
		return err
	}
	if m.Wing == "auto" {
		return errors.New("agent manifest wing must be resolved before saving")
	}
	if !wingPattern.MatchString(m.Wing) {
		return fmt.Errorf("agent manifest wing %q is invalid", m.Wing)
	}
	if m.Capture.MinChars < 0 || m.Capture.MinChars > 10000 {
		return fmt.Errorf("agent manifest capture.min_chars must be between 0 and 10000")
	}
	if m.Recall.BudgetTokens < 0 || m.Recall.BudgetTokens > 10000 {
		return fmt.Errorf("agent manifest recall.budget_tokens must be between 0 and 10000")
	}
	return nil
}

func ResolveProjectWing(projectRoot string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return "", errors.New("project root is required")
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve project root symlinks: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", abs)
	}
	digest := sha256.Sum256([]byte(filepath.Clean(abs)))
	return "project-" + hex.EncodeToString(digest[:])[:12], nil
}
