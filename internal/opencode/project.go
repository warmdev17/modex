package opencode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Native OpenCode shapes (subset). OpenCode v2 carries user text directly on
// the message and assistant parts inside content; tool calls identify
// themselves with id/name and carry output in state.content.
type session struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectID"`
	Agent     string `json:"agent"`
	Model     struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
		Variant    string `json:"variant"`
	} `json:"model"`
	Outcome  string `json:"outcome"`
	Title    string `json:"title"`
	Location struct {
		Directory string `json:"directory"`
	} `json:"location"`
	Time struct {
		Created int64  `json:"created"`
		Updated int64  `json:"updated"`
		Busy    *int64 `json:"busy"`
		Idle    *int64 `json:"idle"`
	} `json:"time"`
}

type message struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Outcome string          `json:"outcome"`
	Agent   string          `json:"agent"`
	Text    string          `json:"text"`
	Content []part          `json:"content"`
	Error   json.RawMessage `json:"error"`
	Finish  string          `json:"finish"`
	Time    struct {
		Created   int64  `json:"created"`
		Completed *int64 `json:"completed"`
	} `json:"time"`
}

type part struct {
	Type string   `json:"type"`
	Text string   `json:"text"`
	Tool toolPart `json:"tool"`
	File struct {
		Mime     string `json:"mime"`
		Filename string `json:"filename"`
		URL      string `json:"url"`
	} `json:"file"`
}

type toolPart struct {
	CallID string    `json:"callID"`
	Tool   string    `json:"tool"`
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	State  toolState `json:"state"`
}

type toolState struct {
	Status  string          `json:"status"`
	Input   json.RawMessage `json:"input"`
	Output  string          `json:"output"`
	Title   string          `json:"title"`
	Error   json.RawMessage `json:"error"`
	Content []toolContent   `json:"content"`
}

type toolContent struct {
	Type string  `json:"type"`
	Text string  `json:"text"`
	URI  string  `json:"uri"`
	Mime string  `json:"mime"`
	Name *string `json:"name"`
}

// modelRef is the flat provider/model pair Modex exposes in models/list.
type modelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

func projectThread(s session) map[string]any {
	thread := map[string]any{
		"id":            s.ID,
		"cwd":           s.Location.Directory,
		"modelProvider": s.Model.ProviderID,
	}
	if s.Model.ID != "" {
		thread["model"] = s.Model.ID
	}
	return thread
}

// projectSessionList omits projectId/projectless deliberately: OpenCode has no
// project concept, so Android falls back to workspace matching.
func projectSessionList(sessions []session) map[string]any {
	data := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		title := safePreview(s.Title)
		entry := projectThread(s)
		entry["preview"] = title
		entry["name"] = title
		data = append(data, entry)
	}
	return map[string]any{"data": data, "next_cursor": nil}
}

// projectMessages mirrors the bridge history entry shape. Text parts of one
// message collapse into a single assistant entry at the first text position;
// reasoning and tool parts stay in content order.
func projectMessages(id, dir string, messages []message) ([]map[string]any, bool, string) {
	entries := make([]map[string]any, 0, len(messages))
	active := false
	activeID := ""
	for _, m := range messages {
		if m.Type == "idle" {
			// An idle marker closes any in-progress assistant response.
			active, activeID = false, ""
			continue
		}
		first := len(entries)
		switch m.Type {
		case "user":
			if text := messageText(m); text != "" {
				entries = append(entries, map[string]any{"id": m.ID, "role": "user", "text": text, "display_text": text})
			}
		case "assistant":
			entries = append(entries, projectAssistant(m, dir)...)
			if messageFailed(m) || terminalOutcome(m.Outcome) || m.Time.Completed != nil {
				active, activeID = false, ""
			} else {
				active, activeID = true, m.ID
			}
		}
		if messageFailed(m) && len(entries) > first {
			last := entries[len(entries)-1]
			notice := map[string]any{
				"category": "turn_failed",
				"message":  "OpenCode could not complete this turn. Review the conversation before trying again.",
				"retrying": false,
			}
			last["status"] = "failed"
			last["turn_notice"] = notice
			// Android reads turn_notice; notice is kept for the documented
			// projection contract as well.
			last["notice"] = notice
		}
	}
	return entries, active, activeID
}

func projectAssistant(m message, dir string) []map[string]any {
	text := assistantText(m)
	entries := make([]map[string]any, 0, len(m.Content)+1)
	emittedText := false
	reasoning := 0
	tools := 0
	for _, p := range m.Content {
		switch p.Type {
		case "text":
			if !emittedText && text != "" {
				entries = append(entries, map[string]any{"id": m.ID, "role": "assistant", "text": text, "display_text": text})
				emittedText = true
			}
		case "reasoning":
			reasoning++
			entries = append(entries, map[string]any{
				"id":           fmt.Sprintf("%s/r%d", m.ID, reasoning),
				"role":         "reasoning",
				"text":         p.Text,
				"display_text": p.Text,
			})
		case "tool":
			tools++
			toolID := p.Tool.CallID
			if toolID == "" {
				toolID = p.Tool.ID
			}
			if toolID == "" {
				toolID = fmt.Sprintf("%s/t%d", m.ID, tools)
			}
			command := p.Tool.Tool
			if command == "" {
				command = p.Tool.Name
			}
			entries = append(entries, map[string]any{
				"id":        toolID,
				"role":      "command",
				"text":      boundedToolOutput(toolOutput(p.Tool)),
				"command":   command,
				"cwd":       dir,
				"status":    toolStatus(p.Tool.State.Status),
				"exit_code": nil,
			})
		}
	}
	if !emittedText && text != "" {
		entries = append(entries, map[string]any{"id": m.ID, "role": "assistant", "text": text, "display_text": text})
	}
	return entries
}

func messageText(m message) string {
	if m.Text != "" {
		return m.Text
	}
	var text strings.Builder
	for _, p := range m.Content {
		if p.Type == "text" {
			text.WriteString(p.Text)
		}
	}
	return text.String()
}

func assistantText(m message) string {
	var text strings.Builder
	for _, p := range m.Content {
		if p.Type == "text" {
			text.WriteString(p.Text)
		}
	}
	if text.Len() == 0 {
		return m.Text
	}
	return text.String()
}

const (
	maxToolOutput = 65536
	truncatedTool = "\n[Output truncated; view full log on host]"
)

func toolOutput(tool toolPart) string {
	if tool.State.Output != "" {
		return tool.State.Output
	}
	var text strings.Builder
	for _, content := range tool.State.Content {
		switch content.Type {
		case "text":
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(content.Text)
		case "file":
			name := content.URI
			if content.Name != nil && *content.Name != "" {
				name = *content.Name
			}
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString("[file: " + name + "]")
		}
	}
	return text.String()
}

func boundedToolOutput(output string) string {
	if len(output) <= maxToolOutput {
		return output
	}
	return strings.ToValidUTF8(output[:maxToolOutput], "") + truncatedTool
}

func toolStatus(status string) string {
	switch status {
	case "completed":
		return "completed"
	case "error", "failed":
		return "failed"
	case "running", "streaming":
		return "inProgress"
	default:
		return "inProgress"
	}
}

func messageFailed(m message) bool {
	if m.Outcome == "error" || m.Outcome == "failed" {
		return true
	}
	if len(m.Error) > 0 && string(m.Error) != "null" {
		return true
	}
	return m.Finish == "error"
}

func terminalOutcome(outcome string) bool {
	switch outcome {
	case "succeeded", "aborted", "error", "failed", "interrupted", "canceled":
		return true
	default:
		return false
	}
}

func safePreview(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return value
}
