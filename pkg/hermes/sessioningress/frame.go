package sessioningress

import (
	"encoding/binary"
	"io"
	"net"
	"time"
	"unicode/utf8"
)

const framePrefixBytes = 4

func EncodeFrame(payload []byte) ([]byte, error) {
	if err := validateFramePayload(payload); err != nil {
		return nil, err
	}
	frame := make([]byte, framePrefixBytes+len(payload))
	binary.BigEndian.PutUint32(frame[:framePrefixBytes], uint32(len(payload)))
	copy(frame[framePrefixBytes:], payload)
	return frame, nil
}

func DecodeFrame(frame []byte) ([]byte, error) {
	if len(frame) < framePrefixBytes {
		return nil, protocolError("truncated frame prefix")
	}
	length := int(binary.BigEndian.Uint32(frame[:framePrefixBytes]))
	if length < 1 || length > MaxFrameBytes {
		return nil, protocolError("frame payload length is outside its allowed range")
	}
	if len(frame) < framePrefixBytes+length {
		return nil, protocolError("truncated frame payload")
	}
	if len(frame) > framePrefixBytes+length {
		return nil, protocolError("trailing frame bytes")
	}
	payload := append([]byte(nil), frame[framePrefixBytes:]...)
	if !utf8.Valid(payload) {
		return nil, protocolError("frame payload is not valid UTF-8")
	}
	return payload, nil
}

func ReadFrame(reader io.Reader) ([]byte, error) {
	var prefix [framePrefixBytes]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return nil, &ProtocolError{Code: "malformed", Err: err}
	}
	length := int(binary.BigEndian.Uint32(prefix[:]))
	if length < 1 || length > MaxFrameBytes {
		return nil, protocolError("frame payload length is outside its allowed range")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, &ProtocolError{Code: "malformed", Err: err}
	}
	if !utf8.Valid(payload) {
		return nil, protocolError("frame payload is not valid UTF-8")
	}
	return payload, nil
}

func WriteFrame(writer io.Writer, payload []byte) error {
	frame, err := EncodeFrame(payload)
	if err != nil {
		return err
	}
	for len(frame) > 0 {
		written, err := writer.Write(frame)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		frame = frame[written:]
	}
	return nil
}

func ReadFrameWithDeadline(conn net.Conn, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		return nil, protocolError("read deadline must be positive")
	}
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	defer conn.SetReadDeadline(time.Time{})
	return ReadFrame(conn)
}

func WriteFrameWithDeadline(conn net.Conn, payload []byte, timeout time.Duration) error {
	if timeout <= 0 {
		return protocolError("write deadline must be positive")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	defer conn.SetWriteDeadline(time.Time{})
	return WriteFrame(conn, payload)
}

func validateFramePayload(payload []byte) error {
	if len(payload) < 1 || len(payload) > MaxFrameBytes {
		return protocolError("frame payload length is outside its allowed range")
	}
	if !utf8.Valid(payload) {
		return protocolError("frame payload is not valid UTF-8")
	}
	return nil
}
