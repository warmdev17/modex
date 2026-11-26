package opencode

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/warmdev/modex/internal/bridge"
)

// Writer serializes framed replies and events to the supervisor connection.
// The SSE stream and command worker can write concurrently.
type Writer struct {
	mu  sync.Mutex
	out io.Writer
	err error
}

func NewWriter(out io.Writer) *Writer {
	return &Writer{out: out}
}

func (w *Writer) writeLine(raw []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err == nil {
		_, w.err = w.out.Write(append(raw, '\n'))
	}
}

func (w *Writer) Emit(method string, params json.RawMessage) {
	if params == nil {
		params = json.RawMessage("null")
	}
	raw, err := json.Marshal(struct {
		Event  string          `json:"event"`
		Params json.RawMessage `json:"params"`
	}{method, params})
	if err == nil {
		w.writeLine(raw)
	}
}

func (w *Writer) Result(id int, result any) {
	raw, err := bridge.EncodeResult(id, result)
	if err != nil {
		raw, _ = bridge.EncodeError(id, err.Error())
	}
	w.writeLine(raw)
}

func (w *Writer) Error(id int, err error) {
	raw, _ := bridge.EncodeError(id, err.Error())
	w.writeLine(raw)
}

func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}
