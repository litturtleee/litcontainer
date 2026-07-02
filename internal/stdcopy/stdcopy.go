package stdcopy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	Stdin     byte = 0
	Stdout    byte = 1
	Stderr    byte = 2
	Exit      byte = 3
	HeaderLen      = 8
)

// Writer 将 stdout / stderr 写入 framed 字节流
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// streamWriter 是 Writer 的包装，携带 stream type 信息
type streamWriter struct {
	*Writer
	stream byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
	}
}

func (w *Writer) Stdout() io.Writer { return &streamWriter{w, Stdout} }
func (w *Writer) Stderr() io.Writer { return &streamWriter{w, Stderr} }
func (w *Writer) Exit() io.Writer   { return &streamWriter{w, Exit} }
func (w *Writer) WriteExit(code int) error {
	exitWriter := w.Exit()
	p := []byte(strconv.Itoa(code))
	_, err := exitWriter.Write(p)
	return err
}

// Write 封装frame
// Frame 格式（8 字节 header + payload）：
//
//	┌───────────┬─────────────┬────────────────┐
//	│ stream(1) │ reserved(3) │  length(4 BE)  │
//	├───────────┴─────────────┴────────────────┤
//	│             payload (length bytes)       │
//	└──────────────────────────────────────────┘
func (s *streamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	var header [HeaderLen]byte
	header[0] = s.stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(p)))

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(header[:]); err != nil {
		return 0, err
	}
	if _, err := s.w.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Demux 从 r 读取 framed 字节流，按 stream type 分发到 stdout / stderr
func Demux(r io.Reader, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	var header [HeaderLen]byte
	// 循环直到EOF
	for {
		_, err := io.ReadFull(r, header[:])
		if err == io.EOF {
			return nil
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("stdcopy: truncated header")
		}
		if err != nil {
			return fmt.Errorf("stdcopy: read header: %w", err)
		}

		stream := header[0]
		length := binary.BigEndian.Uint32(header[4:])
		if length == 0 {
			continue
		}

		var dst io.Writer
		switch stream {
		case Stdout:
			dst = stdout
		case Stderr:
			dst = stderr
		default:
			return fmt.Errorf("stdcopy: unknown stream type %d", stream)
		}

		if _, err := io.CopyN(dst, r, int64(length)); err != nil {
			return fmt.Errorf("stdcopy: read payload (len=%d): %w", length, err)
		}
	}
}

func DemuxExec(r io.Reader, stdout, stderr io.Writer) (int, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	var header [HeaderLen]byte
	for {
		_, err := io.ReadFull(r, header[:])
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return -1, fmt.Errorf("stdcopy: stream ended without exit frame")
			}
			return -1, fmt.Errorf("stdcopy: read header: %w", err)
		}

		stream := header[0]
		length := binary.BigEndian.Uint32(header[4:])

		switch stream {
		case Stdout:
			if length > 0 {
				if _, err := io.CopyN(stdout, r, int64(length)); err != nil {
					return -1, fmt.Errorf("stdcopy: read stdout payload (len=%d): %w", length, err)
				}
			}
		case Stderr:
			if length > 0 {
				if _, err := io.CopyN(stderr, r, int64(length)); err != nil {
					return -1, fmt.Errorf("stdcopy: read stderr payload (len=%d): %w", length, err)
				}
			}
		case Exit:
			payload := make([]byte, length)
			if length > 0 {
				if _, err := io.ReadFull(r, payload); err != nil {
					return -1, fmt.Errorf("stdcopy: read exit payload (len=%d): %w", length, err)
				}
			}
			code, err := strconv.Atoi(strings.TrimSpace(string(payload)))
			if err != nil {
				return -1, fmt.Errorf("stdcopy: parse exit code %q: %w", string(payload), err)
			}
			return code, nil
		default:
			return -1, fmt.Errorf("stdcopy: unknown stream type %d", stream)
		}
	}
}
