package bridge

import "time"

type turnMetadata struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	StartedAt   *int64 `json:"startedAt"`
	CompletedAt *int64 `json:"completedAt"`
	DurationMs  *int64 `json:"durationMs"`
}

func (t turnMetadata) project(message map[string]any) {
	message["turn_id"] = t.ID
	message["turn_status"] = t.Status
	if t.Status != "inProgress" && t.DurationMs != nil && *t.DurationMs >= 0 {
		message["duration_ms"] = *t.DurationMs
	} else if t.Status != "inProgress" && t.StartedAt != nil && t.CompletedAt != nil && *t.StartedAt >= 0 && *t.CompletedAt >= *t.StartedAt && *t.CompletedAt <= 253402300799 {
		message["duration_ms"] = (*t.CompletedAt - *t.StartedAt) * 1000
	}
	if t.Status == "inProgress" && t.StartedAt != nil && *t.StartedAt >= 0 && *t.StartedAt <= time.Now().Unix() {
		message["elapsed_ms"] = (time.Now().Unix() - *t.StartedAt) * 1000
	}
}
