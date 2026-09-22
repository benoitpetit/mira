package agentmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// ToolPrefix is the public MCP namespace for MIRA's built-in identity tools.
// The implementation remains part of MIRA; the name makes the identity
// surface easier to discover without exposing the internal package name.
const ToolPrefix = "soul_"

type Controller struct{ runtime *Runtime }

func NewController(runtime *Runtime) *Controller { return &Controller{runtime: runtime} }

func (c *Controller) ToolDefinitions() []mcptypes.Tool {
	text := func(name, description string, properties map[string]interface{}) mcptypes.Tool {
		return mcptypes.Tool{Name: ToolPrefix + name, Description: description, InputSchema: mcptypes.ToolInputSchema{Type: "object", Properties: properties}}
	}
	str := func(description string) map[string]string {
		return map[string]string{"type": "string", "description": description}
	}
	num := func(description string) map[string]string {
		return map[string]string{"type": "number", "description": description}
	}
	return []mcptypes.Tool{
		text("capture", "Capture and version MIRA's built-in identity from a conversation. The capture is local, deterministic and linked to MIRA memory.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "conversation": str("Conversation or assistant response (required)"), "model_id": str("Current model identifier"), "session_id": str("Session identifier"), "behavioral_metrics": str("Optional JSON object of runtime metrics")}),
		text("recall", "Compose MIRA's identity and relevant MIRA memories inside one bounded token budget.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "context": str("Current conversation context"), "budget": num("Maximum identity context tokens")}),
		text("drift", "Measure identity drift across immutable MIRA identity versions.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "window": num("Number of versions")}),
		text("swap", "Record a model transition and generate bounded identity continuity reinforcement.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "from_model": str("Previous model"), "to_model": str("New model")}),
		text("status", "Return the current MIRA identity summary for an agent.", map[string]interface{}{"agent_id": str("Agent identifier (required)")}),
		text("history", "Return immutable MIRA identity versions for an agent.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "limit": num("Maximum versions")}),
		text("update", "Apply a natural-language identity directive as a new immutable MIRA version.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "directive": str("Directive in French or English"), "reason": str("Reason for the change")}),
		text("patch", "Apply explicit bounded identity fields as a new immutable MIRA version.", map[string]interface{}{"agent_id": str("Agent identifier (required)"), "reason": str("Reason for the change"), "enthusiasm_level": num("0 to 1"), "formality_level": num("0 to 1"), "humor_level": num("0 to 1"), "empathy_level": num("0 to 1"), "technical_depth": num("0 to 1"), "directness_level": num("0 to 1"), "vocabulary_richness": num("0 to 1"), "metaphor_usage": num("0 to 1"), "uses_emojis": map[string]string{"type": "boolean"}, "uses_markdown": map[string]string{"type": "boolean"}, "sentence_structure": str("concise, elaborate, balanced, punchy or flowing")}),
	}
}

func (c *Controller) Call(ctx context.Context, name string, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	if !strings.HasPrefix(name, ToolPrefix) {
		return nil, fmt.Errorf("unknown soul tool %q", name)
	}
	name = strings.TrimPrefix(name, ToolPrefix)
	if c == nil || c.runtime == nil {
		return nil, fmt.Errorf("soul identity is not initialized")
	}
	agentID, err := requiredString(args, "agent_id")
	if err != nil {
		return nil, err
	}
	switch name {
	case "capture":
		conversation, err := requiredString(args, "conversation")
		if err != nil {
			return nil, err
		}
		modelID, _ := args["model_id"].(string)
		sessionID, _ := args["session_id"].(string)
		metrics := map[string]interface{}{}
		if raw, ok := args["behavioral_metrics"].(string); ok && strings.TrimSpace(raw) != "" {
			if err := json.Unmarshal([]byte(raw), &metrics); err != nil {
				return nil, fmt.Errorf("behavioral_metrics must be valid JSON: %w", err)
			}
		}
		snap, err := c.runtime.Capture(ctx, CaptureRequest{AgentID: agentID, Conversation: conversation, ModelID: modelID, SessionID: sessionID, BehavioralMetrics: metrics})
		if err != nil {
			return nil, err
		}
		return textResult(fmt.Sprintf("Identity captured.\nAgent: %s\nVersion: %d\nConfidence: %.1f%%\nTraits: %d\nTimestamp: %s", snap.AgentID, snap.Version, snap.ConfidenceScore*100, len(snap.PersonalityTraits), snap.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))), nil
	case "recall":
		budget := numberArg(args, "budget", 0)
		contextText, _ := args["context"].(string)
		prompt, err := c.runtime.Recall(ctx, agentID, contextText, budget)
		if err != nil {
			return nil, err
		}
		return textResult(fmt.Sprintf("=== MIRA IDENTITY CONTEXT (%d/%d tokens) ===\n%s\n=== END MIRA IDENTITY CONTEXT ===", prompt.TokenEstimate, prompt.BudgetTokens, prompt.Content)), nil
	case "drift":
		report, err := c.runtime.Drift(ctx, agentID, numberArg(args, "window", 0))
		if err != nil {
			return nil, err
		}
		return jsonResult(report)
	case "swap":
		from, err := requiredString(args, "from_model")
		if err != nil {
			return nil, err
		}
		to, err := requiredString(args, "to_model")
		if err != nil {
			return nil, err
		}
		swap, prompt, err := c.runtime.HandleSwap(ctx, agentID, from, to)
		if err != nil {
			return nil, err
		}
		if prompt == nil {
			return textResult(fmt.Sprintf("Model transition recorded: %s -> %s", from, to)), nil
		}
		return textResult(fmt.Sprintf("Model transition recorded: %s -> %s\nIdentity preserved: %v\nDrift: %.3f\n\n%s", from, to, swap.IdentityPreserved, swap.IdentityDrift, prompt.Content)), nil
	case "status":
		snap, err := c.runtime.Latest(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if snap == nil {
			return textResult("No identity captured yet."), nil
		}
		return jsonResult(snap)
	case "history":
		history, err := c.runtime.History(ctx, agentID, numberArg(args, "limit", 0))
		if err != nil {
			return nil, err
		}
		return jsonResult(history)
	case "update":
		directive, err := requiredString(args, "directive")
		if err != nil {
			return nil, err
		}
		reason, _ := args["reason"].(string)
		_, result, err := c.runtime.Update(ctx, agentID, directive, reason)
		if err != nil {
			return nil, err
		}
		return jsonResult(result)
	case "patch":
		patch := map[string]interface{}{}
		for key, value := range args {
			if key != "agent_id" && key != "reason" {
				patch[key] = value
			}
		}
		reason, _ := args["reason"].(string)
		_, result, err := c.runtime.Patch(ctx, agentID, patch, reason)
		if err != nil {
			return nil, err
		}
		return jsonResult(result)
	default:
		return nil, fmt.Errorf("unknown soul tool %q", ToolPrefix+name)
	}
}

func requiredString(args map[string]interface{}, key string) (string, error) {
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return strings.TrimSpace(value), nil
}
func numberArg(args map[string]interface{}, key string, fallback int) int {
	value, ok := args[key]
	if !ok {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case json.Number:
		n, _ := strconv.Atoi(string(v))
		return n
	}
	return fallback
}
func textResult(value string) *mcptypes.CallToolResult {
	return &mcptypes.CallToolResult{Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: value}}}
}
func jsonResult(value interface{}) (*mcptypes.CallToolResult, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return textResult(string(data)), nil
}
