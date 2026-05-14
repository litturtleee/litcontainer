package stdcopy

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
)

// 基础 round-trip：写 stdout + stderr → demux → 内容正确分流
func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)

	if _, err := w.Stdout().Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Stderr().Write([]byte("ERROR\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Stdout().Write([]byte("world\n")); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if err := Demux(&buf, &out, &errb); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello\nworld\n" {
		t.Errorf("stdout = %q, want %q", got, "hello\nworld\n")
	}
	if got := errb.String(); got != "ERROR\n" {
		t.Errorf("stderr = %q, want %q", got, "ERROR\n")
	}
}

// 空输入应该干净返回 nil
func TestDemuxEmpty(t *testing.T) {
	var out, errb bytes.Buffer
	if err := Demux(strings.NewReader(""), &out, &errb); err != nil {
		t.Errorf("expected nil for empty reader, got %v", err)
	}
}

// 截断 header 应该报错
func TestDemuxTruncatedHeader(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Stdout().Write([]byte("hi"))
	// 故意只取前 5 字节
	truncated := buf.Bytes()[:5]
	err := Demux(bytes.NewReader(truncated), io.Discard, io.Discard)
	if err == nil {
		t.Error("expected error for truncated header, got nil")
	}
}

// 截断 payload 应该报错
func TestDemuxTruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	w.Stdout().Write([]byte("hello"))
	// 取 header(8) + 部分 payload(2)
	truncated := buf.Bytes()[:HeaderLen+2]
	err := Demux(bytes.NewReader(truncated), io.Discard, io.Discard)
	if err == nil {
		t.Error("expected error for truncated payload, got nil")
	}
}

// 未知 stream type 应该报错
func TestDemuxUnknownStream(t *testing.T) {
	bad := []byte{
		0x05, 0, 0, 0, 0, 0, 0, 0x01, // stream=5 (非法), len=1
		'x',
	}
	err := Demux(bytes.NewReader(bad), io.Discard, io.Discard)
	if err == nil {
		t.Error("expected error for unknown stream type, got nil")
	}
}

// 大 payload 测试（验证 length 字段正确编解码）
func TestRoundTripLarge(t *testing.T) {
	payload := bytes.Repeat([]byte{'A'}, 1<<20) // 1 MiB
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if _, err := w.Stdout().Write(payload); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Demux(&buf, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", out.Len(), len(payload))
	}
}

// 并发写：两个 goroutine 同时往 stdout / stderr 各写 N 次
// 必须每个 frame 完整不交错（否则 demux 会失败 / 数据错位）
func TestConcurrentWriteNoInterleave(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	const n = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			w.Stdout().Write([]byte("OUT\n"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			w.Stderr().Write([]byte("ERR\n"))
		}
	}()
	wg.Wait()

	var out, errb bytes.Buffer
	if err := Demux(&buf, &out, &errb); err != nil {
		t.Fatalf("demux failed (frame likely corrupted): %v", err)
	}
	wantOut := strings.Repeat("OUT\n", n)
	wantErr := strings.Repeat("ERR\n", n)
	if out.String() != wantOut {
		t.Errorf("stdout corrupted: len=%d want=%d", out.Len(), len(wantOut))
	}
	if errb.String() != wantErr {
		t.Errorf("stderr corrupted: len=%s want=%d", errb.String(), len(wantErr))
	}
}

// 0 长度 payload 应该被跳过
func TestEmptyPayloadFrame(t *testing.T) {
	// 手工构造一个 len=0 的 stdout frame，后跟一个有效 frame
	frame := []byte{
		0x01, 0, 0, 0, 0, 0, 0, 0, // stdout, len=0
		0x01, 0, 0, 0, 0, 0, 0, 0x02, 'h', 'i', // stdout, len=2, "hi"
	}
	var out bytes.Buffer
	if err := Demux(bytes.NewReader(frame), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hi" {
		t.Errorf("got %q, want %q", out.String(), "hi")
	}
}
