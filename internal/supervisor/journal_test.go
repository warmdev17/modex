package supervisor

import (
	"encoding/json"
	"testing"
)

func TestJournalReplayAndRetentionGap(t *testing.T) {
	j := NewJournal(2, 1024)
	for _, s := range []string{`{"id":1}`, `{"id":2}`, `{"id":3}`} {
		if err := j.Append(json.RawMessage(s)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := j.After(0); err == nil {
		t.Fatal("missing retention gap")
	}
	entries, _, err := j.After(1)
	if err != nil || len(entries) != 2 || entries[0].Seq != 2 || entries[1].Seq != 3 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if _, _, err := j.After(4); err == nil {
		t.Fatal("accepted future cursor")
	}
	entries, wake, err := j.After(3)
	if err != nil || len(entries) != 0 {
		t.Fatal(err)
	}
	_ = j.Append(json.RawMessage(`{"event":"done"}`))
	select {
	case <-wake:
	default:
		t.Fatal("reader not woken")
	}
}

func TestJournalByteLimitAndValidation(t *testing.T) {
	j := NewJournal(10, 20)
	if j.Append(json.RawMessage(`broken`)) == nil {
		t.Fatal("invalid JSON accepted")
	}
	if j.Append(json.RawMessage(`{"text":"far too large for this journal"}`)) == nil {
		t.Fatal("oversized frame accepted")
	}
	_ = j.Append(json.RawMessage(`{"id":1}`))
	_ = j.Append(json.RawMessage(`{"id":2}`))
	_ = j.Append(json.RawMessage(`{"id":3}`))
	if _, _, err := j.After(0); err == nil {
		t.Fatal("missing byte retention gap")
	}
}
