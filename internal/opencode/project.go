package opencode

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type nativeProject struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Canonical string   `json:"canonical"`
	Sandboxes []string `json:"sandboxes"`
}

func (p nativeProject) roots() []map[string]string {
	if p.ID == "" || p.ID == "global" || len(p.ID) > 256 || strings.ContainsAny(p.ID, "\r\n\x00") {
		return nil
	}
	var roots []map[string]string
	seen := make(map[string]bool)
	for _, path := range append([]string{p.Canonical}, p.Sandboxes...) {
		if !seen[path] && filepath.IsAbs(path) && len(path) <= 32768 && !strings.ContainsAny(path, "\r\n\x00") {
			roots = append(roots, map[string]string{"path": path})
			seen[path] = true
		}
	}
	return roots
}

// Native OpenCode shapes (subset). OpenCode v2 carries user text directly on
// the message and assistant parts inside content; tool calls identify
// themselves with id/name and carry output in state.content.
type session struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectID"`
	ParentID  string `json:"parentID"`
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
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Outcome  string             `json:"outcome"`
	Agent    string             `json:"agent"`
	Text     string             `json:"text"`
	Content  []part             `json:"content"`
	Files    []nativeAttachment `json:"files,omitempty"`
	Error    json.RawMessage    `json:"error"`
	Finish   string             `json:"finish"`
	Metadata struct {
		TurnID string `json:"modex_turn_id"`
	} `json:"metadata"`
	Time struct {
		Created   int64  `json:"created"`
		Completed *int64 `json:"completed"`
	} `json:"time"`
}

type part struct {
	Type  string     `json:"type"`
	Text  string     `json:"text"`
	ID    string     `json:"id,omitempty"`
	Name  string     `json:"name,omitempty"`
	State *toolState `json:"state,omitempty"`
	Tool  *toolPart  `json:"tool,omitempty"`
	File  struct {
		Mime     string `json:"mime"`
		Filename string `json:"filename"`
		URL      string `json:"url"`
	} `json:"file"`
}

func (p part) toolCall() toolPart {
	if p.Tool != nil {
		return *p.Tool
	}
	call := toolPart{ID: p.ID, Name: p.Name}
	if p.State != nil {
		call.State = *p.State
	}
	return call
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
		thread["model"] = modelChoiceID(s.Model.ProviderID, s.Model.ID)
	}
	if s.ProjectID != "" && s.ProjectID != "global" {
		thread["projectId"] = s.ProjectID
	}
	// An empty model/provider is valid until OpenCode attaches one. Android
	// always sends a model for threads/start and renders an empty provider as
	// "Unknown" via sessionSettingsLabel, so this is not a crash path.
	return thread
}

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
	userID := ""
	for _, m := range messages {
		if m.Type == "idle" {
			// An idle marker closes any in-progress assistant response.
			active, activeID = false, ""
			continue
		}
		first := len(entries)
		switch m.Type {
		case "user":
			userID = m.ID
			if m.Metadata.TurnID != "" {
				userID = m.Metadata.TurnID
			}
			text := messageText(m)
			attachments := projectAttachments(m.Files)
			if text != "" || len(attachments) != 0 {
				entry := map[string]any{"id": m.ID, "role": "user", "text": text, "display_text": text}
				if len(attachments) != 0 {
					entry["attachments"] = attachments
				}
				entries = append(entries, entry)
			}
		case "assistant":
			entries = append(entries, projectAssistant(m, dir)...)
			if messageFailed(m) || terminalOutcome(m.Outcome) || m.Time.Completed != nil {
				active, activeID = false, ""
			} else {
				active, activeID = true, userID
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
			// The live synthesizer emits one reasoning item per part with the
			// same <msgID>/r<N> id. summary_parts keeps a later live delta
			// appending to this section instead of replacing it after reload.
			entries = append(entries, map[string]any{
				"id":            fmt.Sprintf("%s/r%d", m.ID, reasoning),
				"role":          "reasoning",
				"text":          p.Text,
				"display_text":  p.Text,
				"summary_parts": []string{p.Text},
			})
		case "tool":
			tools++
			call := p.toolCall()
			toolID := call.CallID
			if toolID == "" {
				toolID = call.ID
			}
			if toolID == "" {
				toolID = fmt.Sprintf("%s/t%d", m.ID, tools)
			}
			command := call.Tool
			if command == "" {
				command = call.Name
			}
			entries = append(entries, map[string]any{
				"id":        toolID,
				"role":      "command",
				"text":      boundedToolOutput(toolOutput(call)),
				"command":   command,
				"cwd":       dir,
				"status":    toolStatus(call.State.Status),
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
