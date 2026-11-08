package supervisor

import (
	"encoding/json"
	"errors"
	"sync"
)

type Entry struct {
	Seq     uint64          `json:"seq"`
	Message json.RawMessage `json:"message"`
}

// Journal retains a bounded in-memory suffix. Readers never block appends.
type Journal struct {
	mu                   sync.Mutex
	entries              []Entry
	seq                  uint64
	bytes                int
	maxEntries, maxBytes int
	wake                 chan struct{}
}

func NewJournal(maxEntries, maxBytes int) *Journal {
	return &Journal{maxEntries: maxEntries, maxBytes: maxBytes, wake: make(chan struct{})}
}

func (j *Journal) Append(raw json.RawMessage) error {
	if !json.Valid(raw) || len(raw) > j.maxBytes {
		return errors.New("invalid or oversized bridge frame")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.seq++
	j.entries = append(j.entries, Entry{Seq: j.seq, Message: append(json.RawMessage(nil), raw...)})
	j.bytes += len(raw)
	for len(j.entries) > j.maxEntries || j.bytes > j.maxBytes {
		j.bytes -= len(j.entries[0].Message)
		j.entries[0] = Entry{}
		j.entries = j.entries[1:]
	}
	close(j.wake)
	j.wake = make(chan struct{})
	return nil
}

func (j *Journal) After(cursor uint64) ([]Entry, <-chan struct{}, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if cursor > j.seq {
		return nil, nil, errors.New("cursor ahead of supervisor journal")
	}
	if len(j.entries) > 0 && cursor < j.entries[0].Seq-1 {
		return nil, nil, errors.New("replay gap: cursor older than retained events")
	}
	result := make([]Entry, 0)
	for _, entry := range j.entries {
		if entry.Seq > cursor {
			result = append(result, entry)
		}
	}
	return result, j.wake, nil
}
