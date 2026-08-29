// Package llm provides the small OpenAI-compatible model boundary used by the
// harness.  It deliberately exposes an interface so the runtime is testable
// without a network connection or provider SDK.
package llm

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// StructuredClient is the only model capability required by the harness.
type StructuredClient interface {
	ChatJSON(ctx context.Context, messages []Message, out any) error
}

type ImageInput struct {
	URL    string
	Detail string
}

type StructuredVisionClient interface {
	ChatJSONWithImages(ctx context.Context, messages []Message, images []ImageInput, out any) error
}
