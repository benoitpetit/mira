package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/benoitpetit/mira/internal/app"
	"github.com/benoitpetit/mira/internal/config"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/spf13/cobra"
)

// claudeCodeHookInput accepts the common fields supplied by Claude Code and
// Codex UserPromptSubmit and Stop hooks. Extra hook fields are ignored.
type claudeCodeHookInput struct {
	Prompt               string `json:"prompt"`
	UserInput            string `json:"user_input"`
	Message              string `json:"message"`
	LastAssistantMessage string `json:"last_assistant_message"`
	HookEventName        string `json:"hook_event_name"`
	SessionID            string `json:"session_id"`
	ThreadID             string `json:"thread_id"`
}

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hook", Hidden: true, Short: "Internal client hook commands"}
	cmd.AddCommand(newClaudeCodeHookCmd())
	cmd.AddCommand(newCodexHookCmd())
	cmd.AddCommand(newWindsurfHookCmd())
	return cmd
}

func newClaudeCodeHookCmd() *cobra.Command {
	return newPromptHookCmd("claude-code", "claude_code_hook")
}

func newCodexHookCmd() *cobra.Command {
	return newPromptHookCmd("codex", "codex_hook")
}

type windsurfHookInput struct {
	Action   string `json:"agent_action_name"`
	ToolInfo struct {
		UserPrompt string `json:"user_prompt"`
		Response   string `json:"response"`
	} `json:"tool_info"`
}

func newWindsurfHookCmd() *cobra.Command {
	var wing, room string
	var minChars int
	cmd := &cobra.Command{
		Use:          "windsurf",
		Hidden:       true,
		SilenceUsage: true,
		Short:        "Store a Windsurf Cascade hook event",
		RunE: func(_ *cobra.Command, _ []string) error {
			if wing == "" {
				return fmt.Errorf("--wing is required")
			}
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read Windsurf hook input: %w", err)
			}
			var event windsurfHookInput
			if err := json.Unmarshal(raw, &event); err != nil {
				return fmt.Errorf("parse Windsurf hook input: %w", err)
			}
			var role, content, source string
			switch event.Action {
			case "pre_user_prompt":
				role, content, source = "user", event.ToolInfo.UserPrompt, "windsurf_hook"
			case "post_cascade_response":
				role, content, source = "assistant", event.ToolInfo.Response, "windsurf_hook"
			default:
				return nil
			}
			return storeHookMemory("windsurf", source, role, content, wing, room, minChars, "", "")
		},
	}
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing to store automatic prompt memories in (required)")
	cmd.Flags().StringVarP(&room, "room", "r", "", "optional room for automatic prompt memories")
	cmd.Flags().IntVar(&minChars, "min-chars", 20, "minimum Unicode character count for a captured message")
	return cmd
}

func newPromptHookCmd(client, source string) *cobra.Command {
	var wing, room string
	var minChars int
	cmd := &cobra.Command{
		Use:          client,
		Hidden:       true,
		SilenceUsage: true,
		Short:        "Store a prompt or completed-response hook event",
		RunE: func(_ *cobra.Command, _ []string) error {
			if wing == "" {
				return fmt.Errorf("--wing is required")
			}
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read %s hook input: %w", client, err)
			}
			var event claudeCodeHookInput
			if err := json.Unmarshal(raw, &event); err != nil {
				return fmt.Errorf("parse %s hook input: %w", client, err)
			}
			role, content := promptHookMessage(event)
			return storeHookMemory(client, source, role, content, wing, room, minChars, event.SessionID, event.ThreadID)
		},
	}
	cmd.Flags().StringVarP(&wing, "wing", "w", "", "wing to store automatic hook memories in (required)")
	cmd.Flags().StringVarP(&room, "room", "r", "", "optional room for automatic hook memories")
	cmd.Flags().IntVar(&minChars, "min-chars", 20, "minimum Unicode character count for a captured message")
	return cmd
}

func promptHookMessage(event claudeCodeHookInput) (role, content string) {
	if event.HookEventName == "Stop" {
		return "assistant", event.LastAssistantMessage
	}
	content = event.Prompt
	if content == "" {
		content = event.UserInput
	}
	if content == "" {
		content = event.Message
	}
	return "user", content
}

func storeHookMemory(client, source, role, content, wing, room string, minChars int, sessionID, threadID string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var roomRef *string
	if room != "" {
		roomRef = &room
	}
	includeAssistant := role == "assistant"
	inputs, err := interactors.ConversationMemoryInputs([]interactors.ConversationMessage{{Role: role, Content: content}}, wing, roomRef, includeAssistant, minChars)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}
	input := inputs[0]
	input.Metrics["source"] = source
	if sessionID != "" {
		input.Metrics["session_id"] = sessionID
	}
	if threadID != "" {
		input.Metrics["thread_id"] = threadID
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	applyStoragePath(cfg)
	prepareHookConfig(cfg)
	application, err := app.NewApplication(cfg)
	if err != nil {
		return fmt.Errorf("initialise MIRA for %s hook: %w", client, err)
	}
	defer application.Close()
	_, err = application.StoreMemoryUC().Execute(context.Background(), input)
	return err
}

// prepareHookConfig keeps short-lived client hook invocations focused on memory
// capture. Their process must not expose servers or start long-running services.
// Storage, embedding and extraction settings remain those of the project.
func prepareHookConfig(cfg *config.Config) {
	cfg.Metrics.Enabled = false
	cfg.Webhooks.Enabled = false
	cfg.API.Enabled = false
	cfg.Soul.Enabled = false
}
