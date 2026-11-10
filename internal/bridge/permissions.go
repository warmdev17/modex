package bridge

import "encoding/json"

// Project only known permission labels, never filesystem scopes or raw policy objects.
type sessionResult struct {
	Thread   Thread `json:"thread"`
	Model    string `json:"model"`
	Approval string `json:"approval_policy,omitempty"`
	Reviewer string `json:"approvals_reviewer,omitempty"`
	Sandbox  string `json:"sandbox,omitempty"`
}

func projectSession(raw json.RawMessage) (sessionResult, error) {
	var source struct {
		Thread   Thread          `json:"thread"`
		Model    string          `json:"model"`
		Approval json.RawMessage `json:"approvalPolicy"`
		Reviewer string          `json:"approvalsReviewer"`
		Sandbox  struct {
			Type string `json:"type"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		return sessionResult{}, err
	}
	result := sessionResult{Thread: source.Thread, Model: source.Model}
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
