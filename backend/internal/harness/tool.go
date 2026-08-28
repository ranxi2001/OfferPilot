package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ToolRisk string

const (
	ToolRiskRead         ToolRisk = "read"
	ToolRiskExternalRead ToolRisk = "external_read"
)

type ToolHandler func(context.Context, json.RawMessage) (json.RawMessage, error)

// FunctionTool is a deterministic capability that can be granted to selected
// Harness agents. Inputs and outputs are JSON so the permission boundary stays
// explicit even when the caller is Go code rather than a model tool call.
type FunctionTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
	Risk        ToolRisk       `json:"risk"`
	Timeout     time.Duration  `json:"-"`
	Handler     ToolHandler    `json:"-"`
}

func (r *Runtime) RegisterTool(tool FunctionTool) error {
	tool.Name = strings.TrimSpace(tool.Name)
	tool.Description = strings.TrimSpace(tool.Description)
	if !agentIDPattern.MatchString(tool.Name) {
		return errors.New("harness: tool name must be 1-64 letters, digits, '.', '_' or '-'")
	}
	if tool.Handler == nil {
		return fmt.Errorf("harness: tool %q needs a handler", tool.Name)
	}
	if len(tool.InputSchema) == 0 {
		return fmt.Errorf("harness: tool %q needs an input schema", tool.Name)
	}
	if tool.Risk == "" {
		tool.Risk = ToolRiskRead
	}
	if tool.Risk != ToolRiskRead && tool.Risk != ToolRiskExternalRead {
		return fmt.Errorf("harness: tool %q has unsupported risk %q", tool.Name, tool.Risk)
	}
	if tool.Timeout < 0 {
		return fmt.Errorf("harness: tool %q has a negative timeout", tool.Name)
	}
	r.toolsMu.Lock()
	r.tools[tool.Name] = tool
	r.toolsMu.Unlock()
	return nil
}

func (r *Runtime) Tool(name string) (FunctionTool, bool) {
	r.toolsMu.RLock()
	tool, exists := r.tools[name]
	r.toolsMu.RUnlock()
	return tool, exists
}

func (r *Runtime) Tools() []FunctionTool {
	r.toolsMu.RLock()
	result := make([]FunctionTool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	r.toolsMu.RUnlock()
	return result
}

func (r *Runtime) CallToolJSON(ctx context.Context, agentID, toolName string, input, out any) error {
	_, err := r.CallToolJSONTrace(ctx, agentID, toolName, input, out)
	return err
}

// CallToolJSONTrace invokes one allowlisted Function Tool and returns a trace
// identifier. Tool arguments and results are intentionally excluded from trace.
func (r *Runtime) CallToolJSONTrace(ctx context.Context, agentID, toolName string, input, out any) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	agent, exists := r.Agent(agentID)
	if !exists {
		return "", fmt.Errorf("harness: agent %q is not registered", agentID)
	}
	if !agentAllowsTool(agent, toolName) {
		return "", fmt.Errorf("harness: agent %q is not allowed to call tool %q", agentID, toolName)
	}
	tool, exists := r.Tool(toolName)
	if !exists {
		return "", fmt.Errorf("harness: tool %q is not registered", toolName)
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("harness: encode tool %q input: %w", toolName, err)
	}
	if out == nil {
		return "", errors.New("harness: tool output target is required")
	}

	traceID := newTraceID()
	queuedAt := time.Now().UTC()
	r.emit(ctx, TraceEvent{TraceID: traceID, AgentID: agentID, ToolName: toolName, Type: TraceQueued, At: queuedAt})
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		r.emit(ctx, TraceEvent{TraceID: traceID, AgentID: agentID, ToolName: toolName, Type: TraceError, At: time.Now().UTC(), Error: ctx.Err().Error()})
		return traceID, ctx.Err()
	}
	defer func() { <-r.sem }()

	startedAt := time.Now().UTC()
	r.emit(ctx, TraceEvent{
		TraceID: traceID, AgentID: agentID, ToolName: toolName, Type: TraceStarted,
		At: startedAt, Wait: startedAt.Sub(queuedAt),
	})

	callCtx, cancel := toolCallContext(ctx, agent.Timeout, tool.Timeout)
	defer cancel()
	encodedOutput, callErr := tool.Handler(callCtx, encodedInput)
	finishedAt := time.Now().UTC()
	if callErr == nil {
		if len(encodedOutput) == 0 {
			callErr = errors.New("tool returned an empty result")
		} else if err := json.Unmarshal(encodedOutput, out); err != nil {
			callErr = fmt.Errorf("decode tool result: %w", err)
		}
	}
	if callErr != nil {
		r.emit(ctx, TraceEvent{
			TraceID: traceID, AgentID: agentID, ToolName: toolName, Type: TraceError,
			At: finishedAt, Duration: finishedAt.Sub(startedAt), Error: callErr.Error(),
		})
		return traceID, fmt.Errorf("harness: agent %q tool %q: %w", agentID, toolName, callErr)
	}
	r.emit(ctx, TraceEvent{
		TraceID: traceID, AgentID: agentID, ToolName: toolName, Type: TraceSucceeded,
		At: finishedAt, Duration: finishedAt.Sub(startedAt),
	})
	return traceID, nil
}

func agentAllowsTool(agent Agent, toolName string) bool {
	for _, allowed := range agent.Tools {
		if allowed == toolName {
			return true
		}
	}
	return false
}

func toolCallContext(parent context.Context, agentTimeout, toolTimeout time.Duration) (context.Context, context.CancelFunc) {
	timeout := agentTimeout
	if timeout <= 0 || (toolTimeout > 0 && toolTimeout < timeout) {
		timeout = toolTimeout
	}
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}
