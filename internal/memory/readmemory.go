package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/tool"
)

// readMemoryTool reads a saved fact's full body by name. It is stateful (bound
// to one project's Store), so boot constructs it and adds it to the registry.
type readMemoryTool struct{ store Store }

// NewReadMemoryTool returns the `read_memory` tool bound to store. A zero/disabled
// store reports unavailability rather than returning empty results.
func NewReadMemoryTool(store Store) tool.Tool { return readMemoryTool{store: store} }

func (readMemoryTool) Name() string { return "read_memory" }

func (readMemoryTool) Description() string {
	return "Read the full content of a saved fact by name. " +
		"Use this when you need details beyond the one-line description shown in the memory index — " +
		"especially for facts with activation=model_decision, which only include their description in the system prompt. " +
		"The name is the slug from the index: the \"<name>\" in \"[label](<name>.md)\". " +
		"Returns the fact's title, description, activation mode, and full body."
}

func (readMemoryTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Slug of the memory to read, as shown in the index (the \"<name>\" in \"[label](<name>.md)\")."}
		},
		"required": ["name"]
	}`)
}

func (readMemoryTool) ReadOnly() bool { return true }

func (t readMemoryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return "", fmt.Errorf("name is required — use the slug from the memory index")
	}

	// Find the fact by name (slug) in the store.
	for _, m := range t.store.List() {
		if m.Name == name {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("# %s\n\n", displayTitle(m.Title, m.Name)))
			if m.Description != "" {
				b.WriteString(fmt.Sprintf("**Description:** %s\n\n", m.Description))
			}
			b.WriteString(fmt.Sprintf("**Activation:** %s\n", m.Activation))
			if m.Topic != "" {
				b.WriteString(fmt.Sprintf("**Topic:** %s\n", m.Topic))
			}
			b.WriteString(fmt.Sprintf("**Type:** %s\n\n", m.Type))
			b.WriteString(m.Body)
			return b.String(), nil
		}
	}
	return "", fmt.Errorf("memory %q not found — check the slug in the memory index", name)
}
