package bridge

import (
	"encoding/json"
	"errors"
)

type Command struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func DecodeCommand(raw []byte) (Command, error) {
	var cmd Command
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return Command{}, errors.New("invalid Modex command JSON")
	}
	if cmd.ID < 1 {
		return Command{}, errors.New("command id must be positive")
	}
	switch cmd.Method {
	case "client/attached", "projects/list", "models/list", "status", "threads/list", "threads/start", "threads/read", "threads/resume", "threads/takeover", "turn/start", "decision/respond":
		return cmd, nil
	default:
		return cmd, errors.New("unsupported Modex command")
	}
}

func EncodeEvent(method string, params json.RawMessage) ([]byte, error) {
	return json.Marshal(struct {
		Method string          `json:"event"`
		Params json.RawMessage `json:"params"`
	}{method, params})
}

func EncodeResult(id int, result any) ([]byte, error) {
	return json.Marshal(struct {
		ID     int `json:"id"`
		Result any `json:"result"`
	}{id, result})
}

func EncodeError(id int, message string) ([]byte, error) {
	return json.Marshal(struct {
		ID    int `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{id, struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{-32000, message}})
}
