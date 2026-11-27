package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	maxTextFormFields     = 8
	maxFormChoices        = 16
	maxTextAnswerBytes    = 4096
	unsupportedFormNotice = "OpenCode is waiting for an input form that this Modex build cannot display. Resolve it on the computer and retry."
)

var formFieldKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

type nativeForm struct {
	ID        string            `json:"id"`
	SessionID string            `json:"sessionID"`
	Title     string            `json:"title"`
	Fields    []json.RawMessage `json:"fields"`
}

type textFormField struct {
	Key         string          `json:"key"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Required    bool            `json:"required"`
	Hidden      bool            `json:"hidden"`
	Options     json.RawMessage `json:"options"`
	Custom      bool            `json:"custom"`
}

func validFormIdentity(sessionID, formID string) bool {
	return strings.HasPrefix(formID, "frm_") && strings.HasPrefix(sessionID, "ses_") &&
		len(formID)+len(sessionID) < 256 && !strings.ContainsAny(formID+sessionID, "\r\n\x00/")
}

func (d *Decisions) handleTextForm(properties json.RawMessage) bool {
	var event struct {
		Form nativeForm `json:"form"`
	}
	if d.ReplyForm == nil || json.Unmarshal(properties, &event) != nil {
		return false
	}
	form := event.Form
	if !validFormIdentity(form.SessionID, form.ID) {
		return false
	}
	turnID := d.turnID(form.SessionID)
	d.mu.Lock()
	d.formRevision++
	notice := d.addTextFormLocked(form, turnID)
	d.mu.Unlock()
	if notice != "" {
		d.failVisible(form.SessionID, notice)
	}
	return true
}

// shortcut: the existing card sends one string per field and always offers
// Other for choices. Extend the Android contract for fixed or multiple choices.
func (d *Decisions) addTextFormLocked(form nativeForm, turnID string) string {
	id := decisionKey(form.SessionID, form.ID)
	if _, consumed := d.consumed[id]; consumed || d.pending[id] != nil {
		return ""
	}
	if len(form.Fields) == 0 || len(form.Fields) > maxTextFormFields {
		return unsupportedFormNotice
	}
	var keys []string
	questions := make([]map[string]any, 0, len(form.Fields))
	seen := make(map[string]bool, len(form.Fields))
	for _, raw := range form.Fields {
		var field textFormField
		if decodeStrictDecision(raw, &field) != nil || field.Type != "string" ||
			field.Hidden || !formFieldKey.MatchString(field.Key) ||
			field.Key == "constructor" || field.Key == "prototype" || seen[field.Key] {
			return unsupportedFormNotice
		}
		question := decisionText(strings.TrimSpace(field.Title+"\n"+field.Description), 1024)
		if question == "" {
			question = decisionText(form.Title, 1024)
		}
		if question == "" {
			return unsupportedFormNotice
		}
		card := map[string]any{"id": field.Key, "question": question}
		if field.Options != nil {
			options := formChoiceOptions(field.Options)
			if !field.Custom || len(options) == 0 {
				return unsupportedFormNotice
			}
			card["options"] = options
		} else if !field.Required {
			return unsupportedFormNotice
		}
		keys = append(keys, field.Key)
		questions = append(questions, card)
		seen[field.Key] = true
	}
	if len(d.pending) >= 128 || len(d.pending)+len(d.consumed) >= 16384 {
		return pendingCapNotice
	}
	decision := &Decision{
		ID: id, Method: "item/tool/requestUserInput", ThreadID: form.SessionID, TurnID: turnID,
		Context: map[string]json.RawMessage{
			"questions": rawDecisionJSON(questions),
		},
		sessionID: form.SessionID, requestID: form.ID, formKeys: keys,
	}
	d.pending[id] = decision
	d.emitRequiredLocked(decision)
	return ""
}

func formChoiceOptions(raw json.RawMessage) []map[string]string {
	var options []struct {
		Value       string `json:"value"`
		Label       string `json:"label"`
		Description string `json:"description"`
	}
	if decodeStrictDecision(raw, &options) != nil || len(options) == 0 || len(options) > maxFormChoices {
		return nil
	}
	choices := make([]map[string]string, 0, len(options))
	seen := make(map[string]bool, len(options))
	for _, option := range options {
		label := option.Label
		if label == "" || label != option.Value || label != decisionText(label, 256) || seen[label] {
			return nil
		}
		choice := map[string]string{"label": label}
		if description := decisionText(option.Description, 512); description != "" {
			choice["description"] = description
		}
		choices = append(choices, choice)
		seen[label] = true
	}
	return choices
}

func textFormAnswer(keys []string, raw json.RawMessage) (map[string]string, error) {
	var result struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	invalid := errors.New("OpenCode form requires one nonblank text answer per field, up to 4096 bytes each")
	if len(raw) > maxTextFormFields*maxTextAnswerBytes*6+4096 ||
		decodeStrictDecision(raw, &result) != nil || len(result.Answers) != len(keys) {
		return nil, invalid
	}
	answer := make(map[string]string, len(keys))
	for _, key := range keys {
		values := result.Answers[key].Answers
		if len(values) != 1 || len(values[0]) > maxTextAnswerBytes || strings.TrimSpace(values[0]) == "" || strings.ContainsRune(values[0], 0) {
			return nil, invalid
		}
		answer[key] = values[0]
	}
	return answer, nil
}

func (d *Decisions) formEpoch() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.formRevision
}

func (d *Decisions) settleForm(sessionID, formID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.formRevision++
	id := decisionKey(sessionID, formID)
	if pending := d.pending[id]; pending != nil && len(pending.formKeys) != 0 {
		delete(d.pending, id)
		d.consumed[id] = struct{}{}
		d.emitEvent("decision/resolved", map[string]string{"decision_id": id})
	} else if len(d.pending)+len(d.consumed) < 16384 {
		d.consumed[id] = struct{}{}
	}
}

// reconcileForms is called with the original turn pinned by the synthesizer.
// A form event or answer after the GET started invalidates the snapshot.
func (d *Decisions) reconcileForms(sessionID, turnID string, epoch uint64, forms []nativeForm) string {
	current := make(map[string]bool, len(forms))
	for _, form := range forms {
		id := decisionKey(form.SessionID, form.ID)
		if form.SessionID != sessionID || !validFormIdentity(form.SessionID, form.ID) ||
			len(form.Fields) == 0 || current[id] {
			return ""
		}
		for _, raw := range form.Fields {
			var field textFormField
			if json.Unmarshal(raw, &field) != nil || field.Key == "" || field.Type == "" {
				return ""
			}
			// encoding/json accepts null for scalar fields without an error.
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil {
				return ""
			}
			for _, value := range fields {
				if string(value) == "null" {
					return ""
				}
			}
		}
		current[id] = true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.formRevision != epoch {
		return ""
	}
	d.formRevision++
	var stale []string
	for id, pending := range d.pending {
		if pending.sessionID == sessionID && len(pending.formKeys) != 0 &&
			(pending.TurnID == "" || pending.TurnID == turnID) && !current[id] {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	for _, id := range stale {
		delete(d.pending, id)
		d.consumed[id] = struct{}{}
		d.emitEvent("decision/resolved", map[string]string{"decision_id": id})
	}
	for _, form := range forms {
		if notice := d.addTextFormLocked(form, turnID); notice != "" {
			return notice
		}
	}
	return ""
}

func (c *Client) listForms(ctx context.Context, sessionID string) ([]nativeForm, error) {
	var response struct {
		Data []nativeForm `json:"data"`
	}
	path := "/api/session/" + url.PathEscape(sessionID) + "/form"
	if err := c.request(ctx, http.MethodGet, path, nil, nil, &response); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return nil, errInvalidResponse
	}
	return response.Data, nil
}

func (c *Client) ReplyForm(ctx context.Context, sessionID, formID string, answer map[string]string) error {
	path := "/api/session/" + url.PathEscape(sessionID) + "/form/" + url.PathEscape(formID) + "/reply"
	return c.request(ctx, http.MethodPost, path, nil, map[string]any{"answer": answer}, nil)
}
