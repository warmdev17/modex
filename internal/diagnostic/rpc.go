package diagnostic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxFrame = 4 * 1024 * 1024

// RPCError is a valid request rejection, not a broken transport.
type RPCError struct {
	category, steerTarget string
	code                  int
}

func (e *RPCError) SteerTarget() string { return e.steerTarget }
func (e *RPCError) Code() int           { return e.code }

func (*RPCError) Error() string {
	return "app-server rejected request; raw error suppressed to protect credentials"
}

func (e *RPCError) Category() string {
	if e.category == "" {
		return "request_rejected"
	}
	return e.category
}

func NewRPCError(raw json.RawMessage) *RPCError {
	var detail struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(raw, &detail)
	category := classifyRPCError(detail.Code, detail.Message)
	if detail.Message == "no active turn to steer" {
		category = "no_active_turn_to_steer"
	}
	var target string
	if remainder, ok := strings.CutPrefix(detail.Message, "expected active turn id `"); ok {
		if _, actual, ok := strings.Cut(remainder, "` but found `"); ok {
			if id, ok := strings.CutSuffix(actual, "`"); ok && id != "" && len(id) <= 256 && !strings.ContainsAny(id, "`\r\n\x00") {
				target = id
			}
		}
	}
	var code int
	_ = json.Unmarshal(detail.Code, &code)
	return &RPCError{category: category, steerTarget: target, code: code}
}

func classifyRPCError(code json.RawMessage, message string) string {
	if isUsageLimitMessage(message) {
		return "usage_limit_exceeded"
	}
	s := string(code) + " " + message
	for _, known := range []struct{ key, category string }{
		{"forked history still references", "thread_has_dependents"},
		{"attached threads", "thread_has_dependents"},
		{"missing source rollout", "thread_history_missing"},
		{"list_turns is not supported", "history_pagination_unsupported"},
		{"takeover_unmanaged", "takeover_unmanaged"},
		{"takeover_mismatch", "takeover_mismatch"},
		{"takeover_unavailable", "takeover_unavailable"},
		{"takeover_uncertain", "takeover_uncertain"},
		{"takeover_invalid", "takeover_invalid"},
		{"active writer", "active_writer"},
		{"already has an active writer", "active_writer"},
		{"unauthorized", "unauthorized"},
		{"authentication", "unauthorized"},
		{"approval", "approval_required"},
		{"question", "approval_required"},
		{"provider", "model_provider_mismatch"},
		{"model", "model_provider_mismatch"},
		{"thread not found", "thread_not_found"},
		{"not found", "thread_not_found"},
	} {
		if strings.Contains(strings.ToLower(s), known.key) {
			return known.category
		}
	}
	return "request_rejected"
}

func readResponse(reader *bufio.Reader, id int) (json.RawMessage, error) {
	for {
		var line []byte
		for {
			chunk, prefix, err := reader.ReadLine()
			if err != nil {
				return nil, fmt.Errorf("read app-server response: %w", err)
			}
			if len(line)+len(chunk) > maxFrame {
				return nil, errors.New("app-server frame exceeds 4 MiB limit")
			}
			line = append(line, chunk...)
			if !prefix {
				break
			}
		}
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &message) != nil {
			return nil, errors.New("invalid app-server JSON")
		}
		if message.ID == nil {
			continue
		}
		if message.Method != "" {
			return nil, errors.New("unexpected server request during diagnostic")
		}
		if *message.ID != id {
			return nil, errors.New("unexpected app-server response ID")
		}
		if len(message.Error) > 0 && !bytes.Equal(message.Error, []byte("null")) {
			return nil, NewRPCError(message.Error)
		}
		if len(message.Result) == 0 {
			return nil, errors.New("app-server response has no result")
		}
		return message.Result, nil
	}
}

func Call(writer io.Writer, reader *bufio.Reader, id int, method string, params any) (json.RawMessage, error) {
	return CallEvents(writer, reader, id, method, params, nil)
}

func CallEvents(writer io.Writer, reader *bufio.Reader, id int, method string, params any, onNotification func(string, json.RawMessage)) (json.RawMessage, error) {
	if err := json.NewEncoder(writer).Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	return readResponseEvents(reader, id, onNotification)
}

func readResponseEvents(reader *bufio.Reader, id int, onNotification func(string, json.RawMessage)) (json.RawMessage, error) {
	for {
		line, err := readFrame(reader)
		if err != nil {
			return nil, err
		}
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &message) != nil {
			return nil, errors.New("invalid app-server JSON")
		}
		if message.ID == nil {
			if message.Method != "" && onNotification != nil {
				onNotification(message.Method, message.Params)
			}
			continue
		}
		if message.Method != "" {
			return nil, errors.New("unexpected server request during diagnostic")
		}
		if *message.ID != id {
			return nil, errors.New("unexpected app-server response ID")
		}
		if len(message.Error) > 0 && !bytes.Equal(message.Error, []byte("null")) {
			return nil, NewRPCError(message.Error)
		}
		if len(message.Result) == 0 {
			return nil, errors.New("app-server response has no result")
		}
		return message.Result, nil
	}
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, prefix, err := reader.ReadLine()
		if err != nil {
			return nil, fmt.Errorf("read app-server response: %w", err)
		}
		if len(line)+len(chunk) > maxFrame {
			return nil, errors.New("app-server frame exceeds 4 MiB limit")
		}
		line = append(line, chunk...)
		if !prefix {
			return line, nil
		}
	}
}
