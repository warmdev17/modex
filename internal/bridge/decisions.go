package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"sync"
)

// Decision contains review context, never the raw server request envelope.
type Decision struct {
	ID       string                     `json:"decision_id"`
	Method   string                     `json:"method"`
	ThreadID string                     `json:"thread_id"`
	TurnID   string                     `json:"turn_id"`
	ItemID   string                     `json:"item_id"`
	Context  map[string]json.RawMessage `json:"context"`
	upstream json.RawMessage
}

type Decisions struct {
	mu      sync.Mutex
	next    int
	pending map[string]Decision
	respond func(context.Context, json.RawMessage, json.RawMessage) error
}

func NewDecisions(respond func(context.Context, json.RawMessage, json.RawMessage) error) *Decisions {
	return &Decisions{pending: map[string]Decision{}, respond: respond}
}

func (d *Decisions) Add(id json.RawMessage, method string, params json.RawMessage) (Decision, error) {
	var fields []string
	switch method {
	case "item/commandExecution/requestApproval":
		fields = []string{"command", "cwd", "reason", "kind", "additionalPermissions", "networkApprovalContext", "availableDecisions"}
	case "item/fileChange/requestApproval":
		fields = []string{"reason", "grantRoot"}
	case "item/tool/requestUserInput":
		fields = []string{"questions", "isBlocking"}
	default:
		return Decision{}, errors.New("unsupported Codex decision request")
	}
	var meta struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
	}
	var p map[string]json.RawMessage
	if json.Unmarshal(params, &meta) != nil || json.Unmarshal(params, &p) != nil || meta.ThreadID == "" || meta.TurnID == "" || meta.ItemID == "" {
		return Decision{}, errors.New("invalid decision context")
	}
	if method == "item/tool/requestUserInput" {
		var questions []question
		if json.Unmarshal(p["questions"], &questions) != nil || len(questions) == 0 {
			return Decision{}, errors.New("invalid questions")
		}
		seen := map[string]bool{}
		for _, q := range questions {
			if q.ID == "" || q.Question == "" || q.IsSecret || seen[q.ID] {
				return Decision{}, errors.New("unsupported or invalid question")
			}
			seen[q.ID] = true
		}
	}
	// Codex permits string or integer request IDs; never accept null or compound IDs.
	var sid string
	var number int64
	if (json.Unmarshal(id, &sid) != nil || sid == "") && (string(id) == "null" || json.Unmarshal(id, &number) != nil) {
		return Decision{}, errors.New("invalid server request ID")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, pending := range d.pending {
		if bytes.Equal(pending.upstream, id) {
			return Decision{}, errors.New("duplicate server request ID")
		}
	}
	if len(d.pending) >= 128 {
		return Decision{}, errors.New("too many pending decisions")
	}
	d.next++
	event := Decision{ID: strconv.Itoa(d.next), Method: method, ThreadID: meta.ThreadID, TurnID: meta.TurnID, ItemID: meta.ItemID, Context: map[string]json.RawMessage{}, upstream: append(json.RawMessage(nil), id...)}
	for _, key := range fields {
		if value, ok := p[key]; ok {
			event.Context[key] = value
		}
	}
	d.pending[event.ID] = event
	return event, nil
}

type question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	IsSecret bool   `json:"isSecret"`
}

func decodeStrict(raw json.RawMessage, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid decision response")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid decision response")
	}
	return nil
}

func (d *Decisions) Answer(ctx context.Context, id string, result json.RawMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	request, ok := d.pending[id]
	if !ok {
		return errors.New("decision is missing or already resolved")
	}
	if request.Method == "item/tool/requestUserInput" {
		var response struct {
			Answers map[string]struct {
				Answers []string `json:"answers"`
			} `json:"answers"`
		}
		if err := decodeStrict(result, &response); err != nil {
			return err
		}
		var questions []question
		_ = json.Unmarshal(request.Context["questions"], &questions)
		if len(response.Answers) != len(questions) {
			return errors.New("answer every requested question")
		}
		for _, q := range questions {
			if len(response.Answers[q.ID].Answers) == 0 {
				return errors.New("answer every requested question")
			}
		}
	} else {
		var response struct {
			Decision string `json:"decision"`
		}
		if err := decodeStrict(result, &response); err != nil {
			return err
		}
		switch response.Decision {
		case "accept", "decline", "cancel":
		default:
			return errors.New("decision must be accept, decline or cancel")
		}
		if allowed := request.Context["availableDecisions"]; len(allowed) > 0 && string(allowed) != "null" {
			var choices []json.RawMessage
			if json.Unmarshal(allowed, &choices) != nil {
				return errors.New("invalid available decisions")
			}
			found := false
			for _, choice := range choices {
				var value string
				if json.Unmarshal(choice, &value) == nil && value == response.Decision {
					found = true
				}
			}
			if !found {
				return errors.New("decision not offered by Codex")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Consume before writing: an uncertain delivery must never become a replay.
	delete(d.pending, id)
	if err := d.respond(ctx, request.upstream, result); err != nil {
		return errors.New("decision delivery failed; do not replay")
	}
	return nil
}

func (d *Decisions) List() []Decision {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]Decision, 0, len(d.pending))
	for _, item := range d.pending {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Native TUI and app-server share a versioned schema. Let Codex validate native
// result variants while retaining the same first-answer-wins delivery guard.
func (d *Decisions) AnswerNative(ctx context.Context, id string, result json.RawMessage) error {
	return d.AnswerNativeWith(ctx, id, result, d.respond)
}

// Error envelopes share the same atomic delivery guard as successful native answers.
func (d *Decisions) AnswerNativeWith(ctx context.Context, id string, result json.RawMessage, respond func(context.Context, json.RawMessage, json.RawMessage) error) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(result, &object) != nil || object == nil {
		return errors.New("native decision result must be an object")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	request, ok := d.pending[id]
	if !ok {
		return errors.New("decision is missing or already resolved")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(d.pending, id)
	if err := respond(ctx, request.upstream, result); err != nil {
		return errors.New("decision delivery failed; do not replay")
	}
	return nil
}

func (d *Decisions) Resolve(method string, params json.RawMessage) {
	var event struct {
		ThreadID  string          `json:"threadId"`
		RequestID json.RawMessage `json:"requestId"`
		Turn      struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(params, &event) != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, request := range d.pending {
		if request.ThreadID == event.ThreadID && ((method == "serverRequest/resolved" && bytes.Equal(request.upstream, event.RequestID)) || (method == "turn/completed" && request.TurnID == event.Turn.ID)) {
			delete(d.pending, id)
		}
	}
}
