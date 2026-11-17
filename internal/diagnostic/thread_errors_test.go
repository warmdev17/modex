package diagnostic

import (
	"encoding/json"
	"testing"
)

func TestNativeHistoryRefusalsWithNumericRPCCodes(t *testing.T) {
	for _, tc := range []struct{ message, category string }{{"forked history still references it", "thread_has_dependents"}, {"source cannot be deleted while attached threads exist", "thread_has_dependents"}, {"invalid paginated history lineage: missing source rollout", "thread_history_missing"}} {
		raw, _ := json.Marshal(map[string]any{"code": -32600, "message": tc.message})
		if got := NewRPCError(raw).Category(); got != tc.category {
			t.Fatalf("%s: %s", tc.message, got)
		}
	}
}
