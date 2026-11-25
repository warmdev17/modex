package opencode

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Synthesizer turns OpenCode SSE events and polling snapshots into the Codex
// event shapes Android already consumes. All methods are safe for concurrent
// use by the SSE reader and the command worker.
type Synthesizer struct {
	mu         sync.Mutex
	emit       func(method string, params json.RawMessage)
	defaultDir string
	turns      map[string]*turnState
	last       string
}

type turnState struct {
	sessionID string
	userID    string
	dir       string
	startedAt time.Time

	assistantID        string
	assistantStarted   map[string]bool
	text               map[string]string
	finalText          map[string]string
	reasoning          map[string]map[int]string
	reasoningStarted   map[string]bool
	reasoningCompleted map[string]bool
	toolName           map[string]string
	toolsStarted       map[string]bool
	toolsCompleted     map[string]bool

	completed   bool
	failed      bool
	interrupted bool
}

func NewSynthesizer(defaultDir string, emit func(string, json.RawMessage)) *Synthesizer {
	if emit == nil {
		emit = func(string, json.RawMessage) {}
	}
	return &Synthesizer{
		emit:       emit,
		defaultDir: defaultDir,
		turns:      map[string]*turnState{},
	}
}

// StartTurn resets the session state and emits turn/started before any deltas.
func (s *Synthesizer) StartTurn(sessionID, userID, dir string) {
	if sessionID == "" || userID == "" {
		return
	}
	if dir == "" {
		dir = s.defaultDir
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := &turnState{
		sessionID:          sessionID,
		userID:             userID,
		dir:                dir,
		startedAt:          time.Now(),
		assistantStarted:   map[string]bool{},
		text:               map[string]string{},
		finalText:          map[string]string{},
		reasoning:          map[string]map[int]string{},
		reasoningStarted:   map[string]bool{},
		reasoningCompleted: map[string]bool{},
		toolName:           map[string]string{},
		toolsStarted:       map[string]bool{},
		toolsCompleted:     map[string]bool{},
	}
	s.turns[sessionID] = state
	s.last = sessionID
	s.emitEvent("turn/started", map[string]any{"threadId": sessionID, "turn": map[string]any{"id": userID}})
}

// MarkInterrupted records an accepted user interrupt so a subsequent idle
// event finishes the turn as interrupted even if execution.interrupted was
// missed on the stream.
func (s *Synthesizer) MarkInterrupted(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.turns[sessionID]; state != nil && !state.completed {
		state.interrupted = true
	}
}

// TurnID returns the user message id of the session's current/last turn so
// permission decisions can carry the Codex turn id.
func (s *Synthesizer) TurnID(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.turns[sessionID]; state != nil {
		return state.userID
	}
	return ""
}

// ActiveSession reports the latest turn and whether it is still running; the
// polling reconciler uses the former for its idle tick.
func (s *Synthesizer) ActiveSession() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == "" {
		return "", false
	}
	state := s.turns[s.last]
	if state == nil || state.completed {
		// Stop issuing idle reconciliation requests once the turn is terminal;
		// a later StartTurn re-arms the poller.
		return "", false
	}
	return s.last, true
}

type eventData struct {
	SessionID          string          `json:"sessionID"`
	AssistantMessageID string          `json:"assistantMessageID"`
	MessageID          string          `json:"messageID"`
	Part               json.RawMessage `json:"part"`
	Ordinal            int             `json:"ordinal"`
	Delta              string          `json:"delta"`
	Text               string          `json:"text"`
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Content            []toolContent   `json:"content"`
	Error              json.RawMessage `json:"error"`
	Outcome            string          `json:"outcome"`
}

// Handle applies one SSE event. Both v2 (type/data) and v1 (message.part.*)
// names are accepted; v2 is what OpenCode 2.4.x emits.
func (s *Synthesizer) Handle(typ string, properties json.RawMessage) {
	var data eventData
	if len(properties) > 0 && string(properties) != "null" {
		_ = json.Unmarshal(properties, &data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateFor(data)
	if state == nil || state.completed && !isLateToolEvent(typ) {
		return
	}
	switch typ {
	case "session.execution.started":
		// turn/started is emitted by StartTurn once the prompt is accepted.
	case "session.text.started":
		// Defer the agentMessage item until text actually arrives so an empty
		// or tool-only turn does not render an empty "Codex" bubble.
		s.noteAssistant(state, data.AssistantMessageID)
	case "session.text.delta":
		s.ensureAssistantItem(state, data.AssistantMessageID)
		s.appendText(state, data.AssistantMessageID, data.Delta)
	case "session.text.ended":
		s.ensureAssistantItem(state, data.AssistantMessageID)
		s.finishText(state, data.AssistantMessageID, data.Text)
	case "session.reasoning.started":
		s.noteAssistant(state, data.AssistantMessageID)
		if state.reasoning[data.AssistantMessageID] == nil {
			state.reasoning[data.AssistantMessageID] = map[int]string{}
		}
		// Track the started section even before its first delta so turn
		// completion can finalize it.
		if _, ok := state.reasoning[data.AssistantMessageID][data.Ordinal]; !ok {
			state.reasoning[data.AssistantMessageID][data.Ordinal] = ""
		}
	case "session.reasoning.delta":
		s.noteAssistant(state, data.AssistantMessageID)
		s.appendReasoning(state, data.AssistantMessageID, data.Ordinal, data.Delta)
	case "session.reasoning.ended":
		s.noteAssistant(state, data.AssistantMessageID)
		s.finishReasoning(state, data.AssistantMessageID, data.Ordinal, data.Text)
		s.completeReasoning(state, data.AssistantMessageID, data.Ordinal)
	case "session.tool.input.started":
		s.noteAssistant(state, data.AssistantMessageID)
		if data.ID != "" && data.Name != "" {
			state.toolName[data.ID] = data.Name
		}
		s.ensureToolStarted(state, data.ID, data.Name)
	case "session.tool.called":
		s.noteAssistant(state, data.AssistantMessageID)
		name := state.toolName[data.ID]
		if name == "" {
			name = "tool"
		}
		s.ensureToolStarted(state, data.ID, name)
	case "session.tool.success":
		s.noteAssistant(state, data.AssistantMessageID)
		s.completeTool(state, data.ID, joinToolContent(data.Content), "completed")
	case "session.tool.failed":
		s.noteAssistant(state, data.AssistantMessageID)
		s.completeTool(state, data.ID, joinToolContent(data.Content), "failed")
	case "session.execution.succeeded":
		s.complete(state, "completed")
	case "session.execution.interrupted":
		s.complete(state, "interrupted")
	case "session.execution.failed":
		state.failed = true
		s.complete(state, "failed")
	case "session.step.started":
		// A retry starts a fresh step; a prior step failure is no longer terminal.
		state.failed = false
	case "session.step.failed":
		// A step may retry; wait for execution.failed or idle to finish.
		state.failed = true
	case "session.idle":
		switch {
		case state.failed:
			s.complete(state, "failed")
		case state.interrupted:
			s.complete(state, "interrupted")
		default:
			s.complete(state, "completed")
		}
	case "session.error":
		state.failed = true
		s.complete(state, "failed")
	case "message.part.updated":
		s.handleLegacyPart(state, data)
	case "message.updated":
		s.handleLegacyMessage(state, properties)
	case "session.updated":
		s.handleLegacySession(state, properties)
	case "session.compaction.started", "session.compaction.ended", "session.compaction.failed",
		"session.compacted", "permission.asked", "permission.replied", "permission.updated",
		"file.edited", "session.status", "message.part.removed",
		"session.tool.input.delta", "session.tool.input.ended", "session.tool.progress":
		// Tool input deltas are not output; permissions land in M4.
	}
}

// Reconcile fills in terminal state and parts missed by a dropped SSE stream.
// It is idempotent: existing items/outputs are never emitted twice.
func (s *Synthesizer) Reconcile(session session, messages []message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.turns[session.ID]
	if state == nil || state.completed || state.userID == "" {
		return
	}
	start := 0
	for i, item := range messages {
		if item.ID == state.userID {
			start = i + 1
			break
		}
	}
	idleOutcome := ""
	for _, item := range messages[start:] {
		if item.Type == "idle" {
			idleOutcome = item.Outcome
			continue
		}
		if item.Type != "assistant" {
			continue
		}
		s.noteAssistant(state, item.ID)
		if text := assistantText(item); text != "" {
			s.finishText(state, item.ID, text)
		}
		ordinal := 0
		for _, part := range item.Content {
			switch part.Type {
			case "reasoning":
				s.finishReasoning(state, item.ID, ordinal, part.Text)
				ordinal++
			case "tool":
				id := part.Tool.CallID
				if id == "" {
					id = part.Tool.ID
				}
				name := part.Tool.Tool
				if name == "" {
					name = part.Tool.Name
				}
				s.ensureToolStarted(state, id, name)
				status := toolStatus(part.Tool.State.Status)
				if status == "completed" || status == "failed" {
					s.completeTool(state, id, toolOutput(part.Tool), status)
				}
			}
		}
		if messageFailed(item) {
			state.failed = true
		}
		if item.Time.Completed != nil {
			status := "completed"
			switch {
			case state.failed || messageFailed(item) || session.Outcome == "failed":
				status = "failed"
			case session.Outcome == "interrupted":
				status = "interrupted"
			}
			s.complete(state, status)
			return
		}
	}
	if idleOutcome != "" {
		switch idleOutcome {
		case "failed":
			s.complete(state, "failed")
		case "interrupted":
			s.complete(state, "interrupted")
		default:
			s.complete(state, "completed")
		}
		return
	}
	switch session.Outcome {
	case "failed":
		s.complete(state, "failed")
	case "interrupted":
		s.complete(state, "interrupted")
	case "succeeded":
		s.complete(state, "completed")
		return
	}
	if session.Time.Idle != nil && (session.Time.Busy == nil || *session.Time.Idle >= *session.Time.Busy) {
		status := "completed"
		if state.failed {
			status = "failed"
		}
		s.complete(state, status)
	}
}

func (s *Synthesizer) stateFor(data eventData) *turnState {
	sessionID := data.SessionID
	if sessionID == "" && len(data.Part) > 0 {
		var part struct {
			SessionID string `json:"sessionID"`
		}
		_ = json.Unmarshal(data.Part, &part)
		sessionID = part.SessionID
	}
	return s.turns[sessionID]
}

func (s *Synthesizer) emitEvent(method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.emit(method, raw)
}

// noteAssistant records the assistant message without creating a visible
// agentMessage item. Tool-only and reasoning-only turns must not emit an empty
// "Codex" bubble.
func (s *Synthesizer) noteAssistant(state *turnState, messageID string) {
	if messageID != "" {
		state.assistantID = messageID
	}
}

func (s *Synthesizer) ensureAssistantItem(state *turnState, messageID string) {
	if messageID == "" {
		return
	}
	s.noteAssistant(state, messageID)
	if state.assistantStarted[messageID] {
		return
	}
	state.assistantStarted[messageID] = true
	s.emitEvent("item/started", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"item":     map[string]any{"id": messageID, "type": "agentMessage"},
	})
}

func (s *Synthesizer) appendText(state *turnState, messageID, delta string) {
	if messageID == "" || delta == "" {
		return
	}
	s.ensureAssistantItem(state, messageID)
	state.text[messageID] += delta
	s.emitEvent("item/agentMessage/delta", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"itemId":   messageID,
		"delta":    delta,
	})
}

func (s *Synthesizer) finishText(state *turnState, messageID, full string) {
	if messageID == "" || full == "" {
		return
	}
	s.ensureAssistantItem(state, messageID)
	state.finalText[messageID] = full
	emitted := state.text[messageID]
	switch {
	case full == emitted:
		return
	case strings.HasPrefix(full, emitted):
		delta := full[len(emitted):]
		state.text[messageID] = full
		if delta != "" {
			s.emitEvent("item/agentMessage/delta", map[string]any{
				"threadId": state.sessionID,
				"turnId":   state.userID,
				"itemId":   messageID,
				"delta":    delta,
			})
		}
	case emitted == "":
		state.text[messageID] = full
		s.emitEvent("item/agentMessage/delta", map[string]any{
			"threadId": state.sessionID,
			"turnId":   state.userID,
			"itemId":   messageID,
			"delta":    full,
		})
	}
	// A non-prefix replacement is delivered by the final item/completed.
}

func (s *Synthesizer) appendReasoning(state *turnState, messageID string, ordinal int, delta string) {
	if messageID == "" || delta == "" {
		return
	}
	parts := state.reasoning[messageID]
	if parts == nil {
		parts = map[int]string{}
		state.reasoning[messageID] = parts
	}
	parts[ordinal] += delta
	s.emitReasoningDelta(state, messageID, ordinal, delta)
}

func (s *Synthesizer) finishReasoning(state *turnState, messageID string, ordinal int, full string) {
	if messageID == "" || full == "" {
		return
	}
	parts := state.reasoning[messageID]
	if parts == nil {
		parts = map[int]string{}
		state.reasoning[messageID] = parts
	}
	emitted := parts[ordinal]
	switch {
	case full == emitted:
		return
	case strings.HasPrefix(full, emitted):
		delta := full[len(emitted):]
		parts[ordinal] = full
		if delta != "" {
			s.emitReasoningDelta(state, messageID, ordinal, delta)
		}
	case emitted == "":
		parts[ordinal] = full
		s.emitReasoningDelta(state, messageID, ordinal, full)
	}
}

// reasoningItemID matches project.go's 1-based reasoning part id. OpenCode's
// event ordinal is 0-based, so live and reloaded timelines share the same id.
func reasoningItemID(messageID string, ordinal int) string {
	return fmt.Sprintf("%s/r%d", messageID, ordinal+1)
}

func (s *Synthesizer) ensureReasoningItem(state *turnState, messageID string, ordinal int) {
	if messageID == "" {
		return
	}
	id := reasoningItemID(messageID, ordinal)
	if state.reasoningStarted[id] {
		return
	}
	state.reasoningStarted[id] = true
	s.emitEvent("item/started", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"item":     map[string]any{"id": id, "type": "reasoning", "summary": []string{}},
	})
}

// completeReasoning finalizes one summary section with the Codex-shaped
// summary array Android uses to replace in-progress deltas.
func (s *Synthesizer) completeReasoning(state *turnState, messageID string, ordinal int) {
	if messageID == "" {
		return
	}
	id := reasoningItemID(messageID, ordinal)
	if state.reasoningCompleted[id] {
		return
	}
	text := ""
	if parts := state.reasoning[messageID]; parts != nil {
		text = parts[ordinal]
	}
	if text == "" && !state.reasoningStarted[id] {
		// Suppress empty reasoning sections, matching the empty agentMessage
		// behavior: an item with no visible text should not create a bubble.
		state.reasoningCompleted[id] = true
		return
	}
	s.ensureReasoningItem(state, messageID, ordinal)
	state.reasoningCompleted[id] = true
	s.emitEvent("item/completed", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"item":     map[string]any{"id": id, "type": "reasoning", "summary": []string{text}},
	})
}

// flushReasoning finalizes any summary sections left open when the turn ends
// without an explicit reasoning.ended event.
func (s *Synthesizer) flushReasoning(state *turnState) {
	messageIDs := make([]string, 0, len(state.reasoning))
	for messageID := range state.reasoning {
		messageIDs = append(messageIDs, messageID)
	}
	sort.Strings(messageIDs)
	for _, messageID := range messageIDs {
		ordinals := make([]int, 0, len(state.reasoning[messageID]))
		for ordinal := range state.reasoning[messageID] {
			ordinals = append(ordinals, ordinal)
		}
		sort.Ints(ordinals)
		for _, ordinal := range ordinals {
			s.completeReasoning(state, messageID, ordinal)
		}
	}
}

func (s *Synthesizer) emitReasoningDelta(state *turnState, messageID string, ordinal int, delta string) {
	s.ensureReasoningItem(state, messageID, ordinal)
	// Each reasoning part is its own item aligned with project.go; therefore
	// this item carries exactly one summary section at index 0.
	s.emitEvent("item/reasoning/summaryTextDelta", map[string]any{
		"threadId":     state.sessionID,
		"turnId":       state.userID,
		"itemId":       reasoningItemID(messageID, ordinal),
		"summaryIndex": 0,
		"delta":        delta,
	})
}

func (s *Synthesizer) ensureToolStarted(state *turnState, toolID, name string) {
	if toolID == "" {
		return
	}
	if name == "" {
		name = "tool"
	}
	state.toolName[toolID] = name
	if state.toolsStarted[toolID] {
		return
	}
	state.toolsStarted[toolID] = true
	s.emitEvent("item/started", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"item": map[string]any{
			"id":      toolID,
			"type":    "commandExecution",
			"command": name,
			"cwd":     state.dir,
			"status":  "inProgress",
		},
	})
}

func (s *Synthesizer) completeTool(state *turnState, toolID, output, status string) {
	if toolID == "" || state.toolsCompleted[toolID] {
		return
	}
	s.ensureToolStarted(state, toolID, state.toolName[toolID])
	state.toolsCompleted[toolID] = true
	command := state.toolName[toolID]
	if command == "" {
		command = "tool"
	}
	s.emitEvent("item/completed", map[string]any{
		"threadId": state.sessionID,
		"turnId":   state.userID,
		"item": map[string]any{
			"id":               toolID,
			"type":             "commandExecution",
			"command":          command,
			"cwd":              state.dir,
			"status":           status,
			"aggregatedOutput": boundedToolOutput(output),
			"exitCode":         nil,
		},
	})
}

func (s *Synthesizer) complete(state *turnState, status string) {
	if state.completed {
		return
	}
	state.completed = true
	switch status {
	case "failed":
		state.failed = true
	case "interrupted":
		state.interrupted = true
	}
	s.flushReasoning(state)
	if state.assistantID != "" {
		text := state.finalText[state.assistantID]
		if text == "" {
			text = state.text[state.assistantID]
		}
		if text != "" {
			s.ensureAssistantItem(state, state.assistantID)
			s.emitEvent("item/completed", map[string]any{
				"threadId": state.sessionID,
				"turnId":   state.userID,
				"item":     map[string]any{"id": state.assistantID, "type": "agentMessage", "text": text},
			})
		}
	}
	if status == "failed" {
		message := "OpenCode could not complete this turn. Review the conversation before trying again."
		notice := map[string]any{"category": "turn_failed", "message": message, "retrying": false}
		s.emitEvent("error", map[string]any{
			"threadId":  state.sessionID,
			"turnId":    state.userID,
			"willRetry": false,
			"notice":    notice,
			"error":     map[string]string{"message": message},
		})
	}
	duration := time.Since(state.startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	s.emitEvent("turn/completed", map[string]any{
		"threadId": state.sessionID,
		"turn":     map[string]any{"id": state.userID, "status": status, "durationMs": duration},
	})
}

func (s *Synthesizer) handleLegacyPart(state *turnState, data eventData) {
	var part struct {
		ID        string   `json:"id"`
		SessionID string   `json:"sessionID"`
		MessageID string   `json:"messageID"`
		Type      string   `json:"type"`
		Text      string   `json:"text"`
		Tool      toolPart `json:"tool"`
	}
	if len(data.Part) == 0 || json.Unmarshal(data.Part, &part) != nil {
		return
	}
	messageID := part.MessageID
	if messageID == "" {
		messageID = data.AssistantMessageID
	}
	switch part.Type {
	case "text":
		if data.Delta != "" {
			s.appendText(state, messageID, data.Delta)
		} else {
			s.finishText(state, messageID, part.Text)
		}
	case "reasoning":
		s.noteAssistant(state, messageID)
		s.finishReasoning(state, messageID, data.Ordinal, part.Text)
	case "tool":
		s.noteAssistant(state, messageID)
		id := part.Tool.CallID
		if id == "" {
			id = part.Tool.ID
		}
		name := part.Tool.Tool
		if name == "" {
			name = part.Tool.Name
		}
		s.ensureToolStarted(state, id, name)
		status := toolStatus(part.Tool.State.Status)
		if status == "completed" || status == "failed" {
			s.completeTool(state, id, toolOutput(part.Tool), status)
		}
	}
}

func (s *Synthesizer) handleLegacyMessage(state *turnState, properties json.RawMessage) {
	var wrapper struct {
		Info message `json:"info"`
	}
	var item message
	if json.Unmarshal(properties, &item) != nil || item.ID == "" {
		if json.Unmarshal(properties, &wrapper) != nil || wrapper.Info.ID == "" {
			return
		}
		item = wrapper.Info
	}
	if item.ID == "" || item.Type != "assistant" {
		return
	}
	s.noteAssistant(state, item.ID)
	if text := assistantText(item); text != "" {
		s.finishText(state, item.ID, text)
	}
	if item.Time.Completed != nil || terminalOutcome(item.Outcome) || messageFailed(item) {
		if messageFailed(item) || item.Outcome == "failed" || item.Outcome == "error" {
			state.failed = true
		}
		status := "completed"
		if state.failed {
			status = "failed"
		} else if item.Outcome == "aborted" || item.Outcome == "interrupted" {
			status = "interrupted"
		}
		s.complete(state, status)
	}
}

func (s *Synthesizer) handleLegacySession(state *turnState, properties json.RawMessage) {
	var item session
	if json.Unmarshal(properties, &item) != nil || item.ID != state.sessionID {
		return
	}
	switch item.Outcome {
	case "failed":
		s.complete(state, "failed")
	case "interrupted":
		s.complete(state, "interrupted")
	case "succeeded":
		s.complete(state, "completed")
	}
}

func joinToolContent(content []toolContent) string {
	var text strings.Builder
	for _, part := range content {
		switch part.Type {
		case "text":
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(part.Text)
		case "file":
			name := part.URI
			if part.Name != nil && *part.Name != "" {
				name = *part.Name
			}
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString("[file: " + name + "]")
		}
	}
	return text.String()
}

func isLateToolEvent(typ string) bool {
	switch typ {
	case "session.tool.success", "session.tool.failed", "session.execution.succeeded",
		"session.execution.interrupted", "session.execution.failed", "session.idle":
		return true
	default:
		return false
	}
}
