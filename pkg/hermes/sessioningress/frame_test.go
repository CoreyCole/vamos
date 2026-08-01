package sessioningress

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestFrameBoundsAndMalformedInput(t *testing.T) {
	oversizedPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(oversizedPrefix, MaxFrameBytes+1)
	for _, frame := range [][]byte{
		nil,
		{0, 0, 0},
		{0, 0, 0, 0},
		oversizedPrefix,
		{0, 0, 0, 2, 'a'},
		{0, 0, 0, 1, 'a', 'b'},
		{0, 0, 0, 1, 0xff},
	} {
		if _, err := DecodeFrame(frame); err == nil {
			t.Fatalf("accepted invalid frame %x", frame)
		}
	}
	if _, err := EncodeFrame(nil); err == nil {
		t.Fatal("accepted empty payload")
	}
	if _, err := EncodeFrame(bytes.Repeat([]byte{'a'}, MaxFrameBytes+1)); err == nil {
		t.Fatal("accepted oversized payload")
	}
	if _, err := EncodeFrame([]byte{0xff}); err == nil {
		t.Fatal("accepted malformed UTF-8")
	}
	for _, payload := range [][]byte{{'a'}, bytes.Repeat([]byte{'a'}, MaxFrameBytes)} {
		frame, err := EncodeFrame(payload)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeFrame(frame)
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("round trip length %d: %v", len(payload), err)
		}
	}
}

func TestReadWriteFrameHandlesPartialIO(t *testing.T) {
	var destination bytes.Buffer
	writer := &shortWriter{writer: &destination, maximum: 3}
	if err := WriteFrame(writer, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(
		&shortReader{reader: bytes.NewReader(destination.Bytes()), maximum: 2},
	)
	if err != nil || string(got) != "payload" {
		t.Fatalf("read frame = %q, %v", got, err)
	}
}

func TestFrameDeadlines(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	start := time.Now()
	if _, err := ReadFrameWithDeadline(left, 20*time.Millisecond); err == nil {
		t.Fatal("read without data did not time out")
	}
	if time.Since(start) > time.Second {
		t.Fatal("read deadline was not bounded")
	}
	if _, err := ReadFrameWithDeadline(left, 0); err == nil {
		t.Fatal("accepted non-positive read deadline")
	}
	if err := WriteFrameWithDeadline(left, []byte("x"), 0); err == nil {
		t.Fatal("accepted non-positive write deadline")
	}
}

type shortWriter struct {
	writer  *bytes.Buffer
	maximum int
}

func (w *shortWriter) Write(value []byte) (int, error) {
	if len(value) > w.maximum {
		value = value[:w.maximum]
	}
	return w.writer.Write(value)
}

type shortReader struct {
	reader  *bytes.Reader
	maximum int
}

func (r *shortReader) Read(value []byte) (int, error) {
	if len(value) > r.maximum {
		value = value[:r.maximum]
	}
	return r.reader.Read(value)
}
