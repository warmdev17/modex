package bridge

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

type threadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

func threadLifecycle(method string, p map[string]json.RawMessage, cwd string, fetch func(string, any) (json.RawMessage, error)) (any, error) {
	var id string
	if json.Unmarshal(p["thread_id"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n\x00") {
		return nil, errors.New("thread_id must be a nonempty single-line string up to 256 bytes")
	}
	if method == "threads/delete" {
		var confirm bool
		if json.Unmarshal(p["confirm"], &confirm) != nil || !confirm {
			return nil, errors.New("delete requires explicit confirmation")
		}
	}
	raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
	if err != nil {
		return nil, err
	}
	source, err := projectSession(raw)
	if err != nil || source.Thread.ID != id {
		return nil, errors.New("invalid thread response")
	}
	if source.Thread.Cwd != cwd {
		return nil, errors.New("thread outside workspace")
	}
	if method == "threads/fork" {
		return forkThread(id, cwd, source.Thread, fetch)
	}
	raw, err = fetch(strings.Replace(method, "threads/", "thread/", 1), map[string]any{"threadId": id})
	if err != nil {
		return nil, err
	}
	if method == "threads/unarchive" {
		result, err := projectSession(raw)
		if err != nil || result.Thread.ID != id {
			return nil, errors.New("restore result uncertain; refresh before retrying")
		}
		if result.Thread.Cwd != cwd {
			return nil, errors.New("restored thread outside workspace; refresh before retrying")
		}
		return map[string]any{"thread": result.Thread}, nil
	}
	return map[string]any{}, nil
}

func forkThread(id, cwd string, origin Thread, fetch func(string, any) (json.RawMessage, error)) (any, error) {
	if origin.Status == nil || (origin.Status.Type != "idle" && origin.Status.Type != "notLoaded") {
		return nil, errors.New("source thread is active or status unavailable; refresh before forking")
	}
	if err := checkInactiveGoal(id, fetch); err != nil {
		return nil, err
	}
	raw, err := fetch("thread/resume", map[string]any{"threadId": id, "excludeTurns": true})
	if err != nil {
		return nil, err
	}
	source, err := projectSession(raw)
	if err != nil || source.Thread.ID != id || source.Thread.Cwd != cwd || source.Thread.Provider != origin.Provider || !reflect.DeepEqual(source.Thread.ProjectID, origin.ProjectID) {
		return nil, errors.New("fork source changed; refresh before forking")
	}
	if source.Thread.Status == nil || source.Thread.Status.Type != "idle" {
		return nil, errors.New("source thread is active; refresh before forking")
	}
	var session map[string]json.RawMessage
	if json.Unmarshal(raw, &session) != nil || source.Model == "" || source.Thread.Provider == "" {
		return nil, errors.New("source settings unavailable")
	}
	var provider string
	if json.Unmarshal(session["modelProvider"], &provider) != nil || provider != source.Thread.Provider {
		return nil, errors.New("source provider unavailable or changed")
	}
	params := map[string]any{"threadId": id, "cwd": cwd, "excludeTurns": true, "deferGoalContinuation": true, "model": source.Model, "modelProvider": source.Thread.Provider}
	for _, name := range []string{"approvalPolicy", "approvalsReviewer", "sandbox"} {
		var value any
		if json.Unmarshal(session[name], &value) != nil || value == nil {
			return nil, errors.New("source permissions unavailable")
		}
	}
	if err := copyForkPermissions(session, params); err != nil {
		return nil, err
	}
	if source.Effort != "" {
		config, _ := params["config"].(map[string]any)
		if config == nil {
			config = map[string]any{}
		}
		config["model_reasoning_effort"] = source.Effort
		params["config"] = config
	}
	for _, name := range []string{"approvalPolicy", "approvalsReviewer", "runtimeWorkspaceRoots", "serviceTier"} {
		if value := session[name]; len(value) > 0 && string(value) != "null" {
			params[name] = value
		}
	}
	raw, err = fetch("thread/fork", params)
	if err != nil {
		return nil, err
	}
	result, err := projectSession(raw)
	if err != nil || result.Thread.ID == "" || result.Thread.ID == id || result.Thread.Cwd != cwd || result.Thread.Provider != source.Thread.Provider || result.Model != source.Model || result.Effort != source.Effort || !reflect.DeepEqual(result.Thread.ProjectID, source.Thread.ProjectID) {
		return nil, errors.New("fork result uncertain or settings changed; refresh before retrying")
	}
	var created map[string]json.RawMessage
	if json.Unmarshal(raw, &created) != nil {
		return nil, errors.New("invalid fork response; refresh before retrying")
	}
	for _, name := range []string{"approvalPolicy", "approvalsReviewer", "sandbox", "activePermissionProfile", "runtimeWorkspaceRoots", "modelProvider"} {
		var before, after any
		if json.Unmarshal(session[name], &before) != nil && len(session[name]) > 0 {
			return nil, errors.New("source permissions unavailable")
		}
		_ = json.Unmarshal(created[name], &after)
		if (before == nil && (name == "approvalPolicy" || name == "approvalsReviewer" || name == "sandbox")) || !reflect.DeepEqual(before, after) {
			return nil, errors.New("fork permissions changed; refresh before selecting the new thread")
		}
	}
	return result, nil
}

// Copy effective permissions using the same conservative rules as fresh plan threads.
func copyForkPermissions(session map[string]json.RawMessage, params map[string]any) error {
	var profile struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(session["activePermissionProfile"], &profile)
	if profile.ID != "" {
		params["permissions"] = profile.ID
		return nil
	}
	var sandbox struct {
		Type            string   `json:"type"`
		Network         bool     `json:"networkAccess"`
		Roots           []string `json:"writableRoots"`
		ExcludeTmp      bool     `json:"excludeTmpdirEnvVar"`
		ExcludeSlashTmp bool     `json:"excludeSlashTmp"`
	}
	if json.Unmarshal(session["sandbox"], &sandbox) != nil {
		return errors.New("source permissions unavailable")
	}
	switch sandbox.Type {
	case "readOnly":
		if sandbox.Network {
			return errors.New("cannot copy network-enabled read-only permissions")
		}
		params["sandbox"] = "read-only"
	case "dangerFullAccess":
		params["sandbox"] = "danger-full-access"
	case "workspaceWrite":
		params["sandbox"] = "workspace-write"
		params["config"] = map[string]any{"sandbox_workspace_write.writable_roots": sandbox.Roots, "sandbox_workspace_write.network_access": sandbox.Network, "sandbox_workspace_write.exclude_tmpdir_env_var": sandbox.ExcludeTmp, "sandbox_workspace_write.exclude_slash_tmp": sandbox.ExcludeSlashTmp}
	default:
		return errors.New("cannot copy source permissions safely")
	}
	return nil
}

func checkInactiveGoal(id string, fetch func(string, any) (json.RawMessage, error)) error {
	// Resuming a persisted active goal can start a turn.
	raw, err := fetch("thread/goal/get", map[string]any{"threadId": id})
	if err != nil {
		return err
	}
	var goalState struct {
		Goal json.RawMessage `json:"goal"`
	}
	if json.Unmarshal(raw, &goalState) != nil || len(goalState.Goal) == 0 {
		return errors.New("source goal unavailable; refresh before continuing")
	}
	if strings.TrimSpace(string(goalState.Goal)) != "null" {
		var goal struct {
			ThreadID string `json:"threadId"`
			Status   string `json:"status"`
		}
		if json.Unmarshal(goalState.Goal, &goal) != nil || goal.ThreadID != id {
			return errors.New("source goal unavailable; refresh before continuing")
		}
		switch goal.Status {
		case "paused", "blocked", "usageLimited", "budgetLimited", "complete":
		default:
			return errors.New("source goal is active or unavailable; pause it before continuing")
		}
	}
	return nil
}
