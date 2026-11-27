package opencode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// The wire budget accommodates eight 20 MiB files inflated by base64. The
// retained JSON still has the ordinary response cap.
const maxAttachmentWire = 256 << 20

var errResponseTooLarge = errors.New("opencode response exceeds size limit")

// Attachment data is a base64 string in these native response fields. Keep
// their descriptors and all other JSON, without allocating the file contents.
var (
	promptAttachmentPath  = []string{"data", "payload", "files", "[]", "data"}
	historyAttachmentPath = []string{"data", "[]", "files", "[]", "data"}
)

type responseFrame struct {
	path, next int
	object     bool
	key        bool
}

func readAttachmentResponse(body io.Reader, path []string, wireLimit int64) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: wireLimit + 1}
	reader := bufio.NewReaderSize(limited, 32<<10)
	var output bytes.Buffer
	var frames []responseFrame
	nextPath := func() int {
		if len(frames) == 0 {
			return 0
		}
		return frames[len(frames)-1].next
	}
	advance := func(index int, segment string) int {
		if index >= 0 && index < len(path) && path[index] == segment {
			return index + 1
		}
		return -1
	}

	for {
		b, err := reader.ReadByte()
		if err == io.EOF && limited.N > 0 {
			return output.Bytes(), nil // json.Unmarshal validates the retained document.
		}
		if err != nil {
			if limited.N == 0 {
				return nil, errResponseTooLarge
			}
			return nil, errInvalidResponse
		}
		if output.Len() >= maxResponse || len(frames) > 256 {
			return nil, errResponseTooLarge
		}
		output.WriteByte(b)
		switch b {
		case '{', '[':
			at := nextPath()
			frame := responseFrame{path: at, next: -1, object: b == '{', key: b == '{'}
			if !frame.object {
				frame.next = advance(at, "[]")
			}
			frames = append(frames, frame)
		case '}', ']':
			if len(frames) == 0 || frames[len(frames)-1].object != (b == '}') {
				return nil, errInvalidResponse
			}
			frames = frames[:len(frames)-1]
		case ',':
			if len(frames) > 0 && frames[len(frames)-1].object {
				frames[len(frames)-1].key = true
				frames[len(frames)-1].next = -1
			}
		case ':':
			if len(frames) > 0 {
				frames[len(frames)-1].key = false
			}
		case '"':
			var object *responseFrame
			if len(frames) > 0 && frames[len(frames)-1].object {
				object = &frames[len(frames)-1]
			}
			if object != nil && !object.key && object.next == len(path) {
				if err := skipAttachmentBase64(reader); err != nil {
					if limited.N == 0 {
						return nil, errResponseTooLarge
					}
					return nil, err
				}
				output.WriteByte('"')
				continue
			}
			start := output.Len() - 1
			if err := copyResponseString(reader, &output); err != nil {
				if limited.N == 0 && errors.Is(err, errInvalidResponse) {
					return nil, errResponseTooLarge
				}
				return nil, err
			}
			if object != nil && object.key && object.path >= 0 {
				var key string
				if json.Unmarshal(output.Bytes()[start:], &key) != nil {
					return nil, errInvalidResponse
				}
				if key != "[]" {
					object.next = advance(object.path, key)
				}
			}
		}
	}
}

func copyResponseString(reader *bufio.Reader, output *bytes.Buffer) error {
	escaped := false
	for {
		if output.Len() >= maxResponse {
			return errResponseTooLarge
		}
		b, err := reader.ReadByte()
		if err != nil {
			return errInvalidResponse
		}
		output.WriteByte(b)
		if b == '"' && !escaped {
			return nil
		}
		escaped = b == '\\' && !escaped
	}
}

func skipAttachmentBase64(reader *bufio.Reader) error {
	for {
		chunk, err := reader.ReadSlice('"')
		if err == nil {
			chunk = chunk[:len(chunk)-1]
		}
		for _, b := range chunk {
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '=') {
				return errInvalidResponse
			}
		}
		if err == nil {
			return nil
		}
		if err != bufio.ErrBufferFull {
			return errInvalidResponse
		}
	}
}
