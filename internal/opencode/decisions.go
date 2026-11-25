package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Decision is the Codex-shaped pending approval Android renders. Context is
// always rebuilt from known fields; raw OpenCode metadata is never forwarded.
type Decision struct {
	ID       string                     `json:"decision_id"`
	Method   string                     `json:"method"`
	ThreadID string                     `json:"thread_id"`
	TurnID   string                     `json:"turn_id"`
	ItemID   string                     `json:"item_id"`
	Context  map[string]json.RawMessage `json:"context"`

	sessionID string
}

type permissionRequest struct {
	ID        string   `json:"id"`
	SessionID string   `json:"sessionID"`
	Action    string   `json:"action"`
	Resources []string `json:"resources"`
	Save      []string `json:"save"`
	Source    *struct {
		Type      string `json:"type"`
		MessageID string `json:"messageID"`
		ID        string `json:"id"`
	} `json:"source"`
	Message string `json:"message"`
}

// Decisions stores pending OpenCode permissions and answers them through the
// verified v2 reply endpoint.
type Decisions struct {
	mu         sync.Mutex
	emit       func(string, json.RawMessage)
	reply      func(context.Context, string, string, string) error
	turnID     func(string) string
	defaultDir string
	pending    map[string]*Decision
}

func NewDecisions(defaultDir string, emit func(string, json.RawMessage), turnID func(string) string, reply func(context.Context, string, string, string) error) *Decisions {
	if emit == nil {
		emit = func(string, json.RawMessage) {}
	}
	if turnID == nil {
		turnID = func(string) string { return "" }
	}
	return &Decisions{
		emit:       emit,
		reply:      reply,
		turnID:     turnID,
		defaultDir: defaultDir,
		pending:    map[string]*Decision{},
	}
}

// Handle consumes permission SSE events. Form events are deliberately ignored:
// OpenCode 2.4.x has no /api/question endpoint, so Codex-shaped questions are
// unsupported (they are answered with a clear error from Answer).
func (d *Decisions) Handle(typ string, properties json.RawMessage, directory string) {
	switch typ {
	case "permission.asked", "permission.updated":
		var request permissionRequest
		if json.Unmarshal(properties, &request) != nil {
			return
		}
		d.add(request, directory)
	case "permission.replied":
		var event struct {
			RequestID string `json:"requestID"`
		}
		if json.Unmarshal(properties, &event) == nil {
			d.resolve(event.RequestID)
		}
	}
}

func (d *Decisions) add(request permissionRequest, directory string) {
	if request.ID == "" || request.SessionID == "" || request.Action == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addLocked(request, directory)
}

func (d *Decisions) addLocked(request permissionRequest, directory string) {
	if request.ID == "" || request.SessionID == "" || request.Action == "" {
		return
	}
	if _, exists := d.pending[request.ID]; exists {
		return
	}
	if len(d.pending) >= 128 {
		return
	}
	if directory == "" {
		directory = d.defaultDir
	}
	decision := buildPermissionDecision(request, directory, d.turnID(request.SessionID))
	if decision == nil {
		return
	}
	d.pending[decision.ID] = decision
	d.emitRequiredLocked(decision)
}

// resolve removes a request after an independent permission.replied event.
func (d *Decisions) resolve(requestID string) {
	if requestID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.pending[requestID]; !ok {
		return
	}
	delete(d.pending, requestID)
	d.emitEvent("decision/resolved", map[string]string{"decision_id": requestID})
}

// Answer maps Modex accept/decline/cancel to OpenCode once/reject. always is
// intentionally not reachable: a one-shot accept must never persist a grant.
func (d *Decisions) Answer(ctx context.Context, id string, result json.RawMessage) error {
	if looksLikeQuestionResult(result) {
		return errors.New("questions are unsupported by the opencode backend")
	}
	d.mu.Lock()
	decision, ok := d.pending[id]
	if !ok {
		d.mu.Unlock()
		return errors.New("decision is missing or already resolved")
	}
	var response struct {
		Decision string `json:"decision"`
	}
	if err := decodeStrictDecision(result, &response); err != nil {
		d.mu.Unlock()
		return err
	}
	switch response.Decision {
	case "accept", "decline", "cancel":
	default:
		d.mu.Unlock()
		return errors.New("decision must be accept, decline or cancel")
	}
	if !decisionOffers(decision.Context, response.Decision) {
		d.mu.Unlock()
		return errors.New("decision not offered by OpenCode")
	}
	delete(d.pending, id)
	sessionID := decision.sessionID
	d.mu.Unlock()

	reply := "reject"
	if response.Decision == "accept" {
		reply = "once"
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.reply(ctx, sessionID, id, reply); err != nil {
		// The decision was consumed before delivery; never replay it.
		return errors.New("decision delivery failed; do not replay")
	}
	d.mu.Lock()
	d.emitEvent("decision/resolved", map[string]string{"decision_id": id})
	d.mu.Unlock()
	return nil
}

// Replay mirrors the Codex attach behavior: clear the phone's cards, then
// re-emit every still-pending request.
func (d *Decisions) Replay(attached bool) {
	if !attached {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.emitEvent("decision/reset", map[string]any{})
	d.emitPendingLocked()
}

// Reconcile aligns the pending store with a GET /permission snapshot, covering
// missed SSE events and requests resolved elsewhere.
func (d *Decisions) Reconcile(sessionID, directory string, raw json.RawMessage) {
	var response struct {
		Data []permissionRequest `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	current := map[string]bool{}
	for _, request := range response.Data {
		if request.SessionID == "" {
			request.SessionID = sessionID
		}
		if request.SessionID != sessionID {
			continue
		}
		current[request.ID] = true
		d.addLocked(request, directory)
	}
	ids := make([]string, 0, len(d.pending))
	for id := range d.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		request := d.pending[id]
		if request.sessionID != sessionID || current[id] {
			continue
		}
		delete(d.pending, id)
		d.emitEvent("decision/resolved", map[string]string{"decision_id": id})
	}
}

// Pending returns a snapshot for tests and diagnostics.
func (d *Decisions) Pending() []Decision {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]Decision, 0, len(d.pending))
	for _, decision := range d.pending {
		result = append(result, *decision)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (d *Decisions) emitPendingLocked() {
	ids := make([]string, 0, len(d.pending))
	for id := range d.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		d.emitRequiredLocked(d.pending[id])
	}
}

func (d *Decisions) emitRequiredLocked(decision *Decision) {
	d.emitEvent("decision/required", decision)
}

func (d *Decisions) emitEvent(method string, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	d.emit(method, raw)
}

func buildPermissionDecision(request permissionRequest, directory, turnID string) *Decision {
	if request.ID == "" || request.SessionID == "" {
		return nil
	}
	kind := permissionKind(request.Action)
	method := "item/commandExecution/requestApproval"
	available := []string{"accept", "decline", "cancel"}
	if kind == "write" || kind == "external_directory" {
		method = "item/fileChange/requestApproval"
		available = []string{"decline", "cancel"}
	}
	resources := sanitizeResources(request.Resources)
	context := map[string]json.RawMessage{
		"kind":               rawDecisionJSON(kind),
		"availableDecisions": rawDecisionJSON(available),
	}
	if command := decisionText(strings.Join(resources, "\n"), 8192); command != "" {
		context["command"] = rawDecisionJSON(command)
	}
	if directory != "" {
		context["cwd"] = rawDecisionJSON(directory)
	}
	reason := decisionText(request.Message, 512)
	if reason == "" {
		reason = "OpenCode is waiting for permission to run this action."
	}
	context["reason"] = rawDecisionJSON(reason)
	if len(request.Save) > 0 {
		saves := sanitizeResources(request.Save)
		if len(saves) > 16 {
			saves = saves[:16]
		}
		if len(saves) > 0 {
			context["additionalPermissions"] = rawDecisionJSON(saves)
		}
	}
	if network := networkApprovalContext(resources); network != nil {
		context["networkApprovalContext"] = rawDecisionJSON(network)
	}
	if grant := grantRoot(resources, kind); grant != "" {
		context["grantRoot"] = rawDecisionJSON(grant)
	}
	itemID := ""
	if request.Source != nil {
		itemID = decisionText(request.Source.ID, 256)
	}
	return &Decision{
		ID:        request.ID,
		Method:    method,
		ThreadID:  request.SessionID,
		TurnID:    turnID,
		ItemID:    itemID,
		Context:   context,
		sessionID: request.SessionID,
	}
}

func permissionKind(action string) string {
	value := strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(value, "bash"), strings.Contains(value, "shell"), strings.Contains(value, "command"), strings.Contains(value, "exec"):
		return "shell"
	case strings.Contains(value, "edit"), strings.Contains(value, "write"), strings.Contains(value, "patch"):
		return "write"
	case strings.Contains(value, "read"):
		return "read"
	case strings.Contains(value, "web"), strings.Contains(value, "fetch"):
		return "web"
	case strings.Contains(value, "task"), strings.Contains(value, "agent"), strings.Contains(value, "subagent"):
		return "subagent"
	case strings.Contains(value, "external"), strings.Contains(value, "directory"):
		return "external_directory"
	case strings.Contains(value, "doom"):
		return "doom_loop"
	default:
		return decisionText(value, 64)
	}
}

func sanitizeResources(resources []string) []string {
	if len(resources) > 64 {
		resources = resources[:64]
	}
	out := make([]string, 0, len(resources))
	for _, resource := range resources {
		if value := decisionText(resource, 1024); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func decisionText(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}

func networkApprovalContext(resources []string) map[string]any {
	for _, resource := range resources {
		parsed, err := url.Parse(resource)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		var port int
		switch parsed.Scheme {
		case "https":
			port = 443
		case "http":
			port = 80
		default:
			continue
		}
		if explicit := parsed.Port(); explicit != "" {
			parsedPort, err := strconv.Atoi(explicit)
			if err != nil || parsedPort < 1 || parsedPort > 65535 {
				continue
			}
			port = parsedPort
		}
		return map[string]any{"host": parsed.Hostname(), "port": port}
	}
	return nil
}

func grantRoot(resources []string, kind string) string {
	if kind != "write" && kind != "external_directory" {
		return ""
	}
	if len(resources) == 0 {
		return ""
	}
	root := resources[0]
	if kind == "write" && filepath.IsAbs(root) {
		if dir := filepath.Dir(root); dir != "." && dir != "/" {
			return decisionText(dir, 4096)
		}
	}
	return decisionText(root, 4096)
}

func rawDecisionJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return raw
}

func decisionOffers(context map[string]json.RawMessage, choice string) bool {
	raw, ok := context["availableDecisions"]
	if !ok {
		return true
	}
	var allowed []string
	if json.Unmarshal(raw, &allowed) != nil {
		return false
	}
	for _, value := range allowed {
		if value == choice {
			return true
		}
	}
	return false
}

func looksLikeQuestionResult(result json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(result, &object) != nil {
		return false
	}
	_, answers := object["answers"]
	_, decision := object["decision"]
	return answers && !decision
}

func decodeStrictDecision(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("invalid decision response")
	}
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
