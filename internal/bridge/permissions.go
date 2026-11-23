package bridge

import "encoding/json"

// Project only known permission labels, never filesystem scopes or raw policy objects.
type sessionResult struct {
	Mode     string `json:"collaboration_mode,omitempty"`
	Thread   Thread `json:"thread"`
	Model    string `json:"model"`
	Effort   string `json:"effort,omitempty"`
	Approval string `json:"approval_policy,omitempty"`
	Reviewer string `json:"approvals_reviewer,omitempty"`
	Sandbox  string `json:"sandbox,omitempty"`
	Profile  string `json:"permission_profile,omitempty"`
}

func projectSession(raw json.RawMessage) (sessionResult, error) {
	var source struct {
		Mode struct {
			Mode string `json:"mode"`
		} `json:"collaborationMode"`
		Thread   Thread          `json:"thread"`
		Model    string          `json:"model"`
		Effort   string          `json:"reasoningEffort"`
		Approval json.RawMessage `json:"approvalPolicy"`
		Reviewer string          `json:"approvalsReviewer"`
		Profile  struct {
			ID string `json:"id"`
		} `json:"activePermissionProfile"`
		Sandbox struct {
			Type string `json:"type"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return sessionResult{}, err
	}
	result := sessionResult{Mode: source.Mode.Mode, Thread: source.Thread, Model: source.Model, Effort: source.Effort}
	if validPermissionID(source.Profile.ID) {
		result.Profile = source.Profile.ID
	}
	if result.Effort == "" {
		result.Effort = source.Thread.Effort
	}
	var approval string
	_ = json.Unmarshal(source.Approval, &approval)
	switch approval {
	case "untrusted", "on-failure", "on-request", "never":
		result.Approval = approval
	}
	switch source.Reviewer {
	case "user", "auto_review", "guardian_subagent":
		result.Reviewer = source.Reviewer
	}
	switch source.Sandbox.Type {
	case "readOnly", "workspaceWrite", "dangerFullAccess", "externalSandbox":
		result.Sandbox = source.Sandbox.Type
	}
	return result, nil
}
