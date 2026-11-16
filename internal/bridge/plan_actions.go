package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

// Plan confirmation is client UI, not a Codex requestUserInput RPC.
// Receipts claim a plan before dispatch; uncertain writes must never be replayed.
type PlanActions struct {
	mu      sync.Mutex
	dir     string
	claimed map[string]bool
	call    Caller
}

func NewPlanActions(dir string, call Caller) *PlanActions {
	return &PlanActions{dir: dir, call: call, claimed: map[string]bool{}}
}

func planReceiptKey(thread, turn, item string) string {
	sum := sha256.Sum256([]byte(thread + "\x00" + turn + "\x00" + item))
	return fmt.Sprintf("%x", sum)
}

func (a *PlanActions) Handled(thread, turn, item string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.handled(planReceiptKey(thread, turn, item))
}
func (a *PlanActions) handled(key string) (bool, error) {
	if a.claimed[key] {
		return true, nil
	}
	if a.dir == "" {
		return false, nil
	}
	info, err := os.Lstat(filepath.Join(a.dir, key+".json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return false, errors.New("unsafe plan receipt")
	}
	return true, nil
}
func (a *PlanActions) claim(key, action string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	seen, err := a.handled(key)
	if err != nil {
		return err
	}
	if seen {
		return errors.New("plan already handled; refresh before continuing")
	}
	if a.dir != "" {
		if err = os.MkdirAll(a.dir, 0700); err != nil {
			return err
		}
		info, err := os.Lstat(a.dir)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return errors.New("unsafe plan receipt directory")
		}
		f, err := os.OpenFile(filepath.Join(a.dir, key+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("plan already claimed or receipt unavailable")
		}
		a.claimed[key] = true
		data, _ := json.Marshal(map[string]string{"action": action})
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	a.claimed[key] = true
	return nil
}

func (a *PlanActions) Respond(ctx context.Context, p map[string]any) (json.RawMessage, error) {
	text := func(k string) string { s, _ := p[k].(string); return s }
	thread, turn, item, cwd, action := text("threadId"), text("turnId"), text("itemId"), text("cwd"), text("action")
	if thread == "" || turn == "" || item == "" || !filepath.IsAbs(cwd) || (action != "implement" && action != "fresh" && action != "stay") {
		return nil, errors.New("invalid plan action")
	}
	key := planReceiptKey(thread, turn, item)
	if handled, err := a.Handled(thread, turn, item); err != nil || handled {
		return nil, errors.New("plan already handled or unavailable; refresh before continuing")
	}
	raw, err := a.call(ctx, "thread/resume", map[string]any{"threadId": thread, "excludeTurns": true})
	if err != nil {
		return nil, err
	}
	var session map[string]json.RawMessage
	var origin struct {
		ID        string  `json:"id"`
		Cwd       string  `json:"cwd"`
		ProjectID *string `json:"projectId"`
		Status    struct {
			Type string `json:"type"`
		} `json:"status"`
	}
	if json.Unmarshal(raw, &session) != nil || json.Unmarshal(session["thread"], &origin) != nil || origin.ID != thread || origin.Cwd != cwd {
		return nil, errors.New("plan thread outside workspace")
	}
	if origin.Status.Type != "" && origin.Status.Type != "idle" {
		return nil, errors.New("plan turn is active; refresh")
	}
	source := append(json.RawMessage(nil), raw...)
	page, err := a.call(ctx, "thread/turns/list", map[string]any{"threadId": thread, "limit": 1, "sortDirection": "desc", "itemsView": "full"})
	if err != nil {
		return nil, err
	}
	var history struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Items  []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(page, &history) != nil || len(history.Data) != 1 || history.Data[0].ID != turn || history.Data[0].Status != "completed" {
		return nil, errors.New("plan is stale or turn is active; refresh")
	}
	planID, markdown := "", ""
	for _, entry := range history.Data[0].Items {
		if entry.Type == "plan" {
			planID, markdown = entry.ID, entry.Text
		}
	}
	if planID != item || strings.TrimSpace(markdown) == "" {
		return nil, errors.New("completed plan is unavailable; refresh")
	}
	var model, effort string
	_ = json.Unmarshal(session["model"], &model)
	_ = json.Unmarshal(session["reasoningEffort"], &effort)
	if chosen := text("model"); chosen != "" {
		model = chosen
	}
	if chosen := text("effort"); chosen != "" {
		effort = chosen
	}
	if model == "" {
		return nil, errors.New("source model is unavailable")
	}
	// A fresh thread must retain the actual permission profile, not host defaults.
	fresh := map[string]any{}
	if action == "fresh" {
		var profile struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(session["activePermissionProfile"], &profile)
		fresh = map[string]any{"cwd": cwd, "projectId": origin.ProjectID, "sessionStartSource": "clear"}
		if profile.ID != "" {
			fresh["permissions"] = profile.ID
		} else {
			var sandbox struct {
				Type            string   `json:"type"`
				Network         bool     `json:"networkAccess"`
				Roots           []string `json:"writableRoots"`
				ExcludeTmp      bool     `json:"excludeTmpdirEnvVar"`
				ExcludeSlashTmp bool     `json:"excludeSlashTmp"`
			}
			if json.Unmarshal(session["sandbox"], &sandbox) != nil {
				return nil, errors.New("source permissions unavailable")
			}
			switch sandbox.Type {
			case "readOnly":
				if sandbox.Network {
					return nil, errors.New("cannot copy network-enabled read-only permissions")
				}
				fresh["sandbox"] = "read-only"
			case "dangerFullAccess":
				fresh["sandbox"] = "danger-full-access"
			case "workspaceWrite":
				fresh["sandbox"] = "workspace-write"
				fresh["config"] = map[string]any{"sandbox_workspace_write.writable_roots": sandbox.Roots, "sandbox_workspace_write.network_access": sandbox.Network, "sandbox_workspace_write.exclude_tmpdir_env_var": sandbox.ExcludeTmp, "sandbox_workspace_write.exclude_slash_tmp": sandbox.ExcludeSlashTmp}
			default:
				return nil, errors.New("cannot copy source permissions safely")
			}
		}
		for _, name := range []string{"modelProvider", "approvalPolicy", "approvalsReviewer", "runtimeWorkspaceRoots", "serviceTier", "multiAgentMode"} {
			if value := session[name]; len(value) > 0 && string(value) != "null" {
				fresh[name] = value
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = a.claim(key, action); err != nil {
		return nil, err
	}
	result := map[string]any{"action": action, "source_thread_id": thread, "plan_key": turn + ":" + item}
	if action == "stay" {
		result["thread_id"] = thread
		return json.Marshal(result)
	}
	prompt := "Implement the plan."
	if action == "fresh" {
		raw, err = a.call(ctx, "thread/start", fresh)
		if err != nil {
			return nil, err
		}
		var created map[string]json.RawMessage
		var target struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(raw, &created) != nil || json.Unmarshal(created["thread"], &target) != nil || target.ID == "" || target.ID == thread || target.Cwd != cwd {
			return nil, errors.New("fresh thread result uncertain; do not retry")
		}
		result["thread_id"] = target.ID
		for _, name := range []string{"approvalPolicy", "approvalsReviewer", "sandbox", "activePermissionProfile"} {
			var before, after any
			_ = json.Unmarshal(session[name], &before)
			_ = json.Unmarshal(created[name], &after)
			if (before == nil && name != "activePermissionProfile") || !reflect.DeepEqual(before, after) {
				return nil, errors.New("fresh thread permissions changed; no implementation turn submitted")
			}
		}
		source = raw
		thread = target.ID
		prepared, prepareErr := a.call(ctx, "thread/prepareApprovedPlan", map[string]any{"threadId": thread, "markdown": markdown, "attachmentPath": nil})
		if prepareErr != nil {
			return nil, prepareErr
		}
		var preparation struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(prepared, &preparation) != nil || strings.TrimSpace(preparation.Prompt) == "" {
			return nil, errors.New("invalid prepared plan; no implementation submitted")
		}
		prompt = preparation.Prompt
	}
	settings := map[string]any{"model": model, "reasoning_effort": nil, "developer_instructions": nil}
	if effort != "" {
		settings["reasoning_effort"] = effort
	}
	started, err := a.call(ctx, "turn/start", map[string]any{"threadId": thread, "input": []map[string]string{{"type": "text", "text": prompt}}, "collaborationMode": map[string]any{"mode": "default", "settings": settings}})
	if err != nil {
		return nil, err
	}
	projected, err := projectSession(source)
	if err != nil {
		return nil, err
	}
	projected.Mode = "default"
	projected.Model = model
	projected.Effort = effort
	result["session"] = projected
	result["thread_id"] = thread
	result["turn_result"] = started
	return json.Marshal(result)
}
