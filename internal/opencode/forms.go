package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var formFieldKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

// shortcut: a single required free-text field fits the existing input card.
// Extend the card and validation together before accepting other field shapes.
func (d *Decisions) handleTextForm(properties json.RawMessage) bool {
	var event struct {
		Form struct {
			ID        string            `json:"id"`
			SessionID string            `json:"sessionID"`
			Title     string            `json:"title"`
			Fields    []json.RawMessage `json:"fields"`
		} `json:"form"`
	}
	if d.ReplyForm == nil || json.Unmarshal(properties, &event) != nil || len(event.Form.Fields) != 1 {
		return false
	}
	form := event.Form
	if !strings.HasPrefix(form.ID, "frm_") || !strings.HasPrefix(form.SessionID, "ses_") ||
		len(form.ID)+len(form.SessionID) >= 256 || strings.ContainsAny(form.ID+form.SessionID, "\r\n\x00/") {
		return false
	}
	var field struct {
		Key         string `json:"key"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Required    bool   `json:"required"`
		Hidden      bool   `json:"hidden"`
	}
	if decodeStrictDecision(form.Fields[0], &field) != nil || field.Type != "string" ||
		!field.Required || field.Hidden || !formFieldKey.MatchString(field.Key) ||
		field.Key == "constructor" || field.Key == "prototype" {
		return false
	}
	question := decisionText(strings.TrimSpace(field.Title+"\n"+field.Description), 1024)
	if question == "" {
		question = decisionText(form.Title, 1024)
	}
	if question == "" {
		return false
	}
	turnID := d.turnID(form.SessionID)
	id := decisionKey(form.SessionID, form.ID)
	d.mu.Lock()
	if _, consumed := d.consumed[id]; consumed || d.pending[id] != nil {
		d.mu.Unlock()
		return true
	}
	if len(d.pending) >= 128 || len(d.pending)+len(d.consumed) >= 16384 {
		d.mu.Unlock()
		d.failPendingCap(form.SessionID)
		return true
	}
	decision := &Decision{
		ID: id, Method: "item/tool/requestUserInput", ThreadID: form.SessionID, TurnID: turnID,
		Context: map[string]json.RawMessage{
			"questions": rawDecisionJSON([]map[string]string{{"id": field.Key, "question": question}}),
		},
		sessionID: form.SessionID, requestID: form.ID, formKey: field.Key,
	}
	d.pending[id] = decision
	d.emitRequiredLocked(decision)
	d.mu.Unlock()
	return true
}

func textFormAnswer(key string, raw json.RawMessage) (map[string]string, error) {
	var result struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if len(raw) <= 32768 && decodeStrictDecision(raw, &result) == nil && len(result.Answers) == 1 {
		values := result.Answers[key].Answers
		if len(values) == 1 && len(values[0]) <= 4096 && strings.TrimSpace(values[0]) != "" && !strings.ContainsRune(values[0], 0) {
			return map[string]string{key: values[0]}, nil
		}
	}
	return nil, errors.New("OpenCode form requires one nonblank text answer up to 4096 bytes")
}

func (c *Client) ReplyForm(ctx context.Context, sessionID, formID string, answer map[string]string) error {
	path := "/api/session/" + url.PathEscape(sessionID) + "/form/" + url.PathEscape(formID) + "/reply"
	return c.request(ctx, http.MethodPost, path, nil, map[string]any{"answer": answer}, nil)
}
