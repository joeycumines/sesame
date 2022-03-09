package grpc

import (
	"io"
	"sync"
	"sync/atomic"
)

type (
	// Reader implements support for using a gRPC stream as an io.Reader, where the caller provides the
	// implementation to transform received messages (in an arbitrary manner) into a binary stream.
	//
	// It is not intended to be used in cases where the binary stream might be multiplexed with unhandled messages.
	// This is related to limitations of the behavior, chosen for simplicity. Specifically, Read will return io.EOF
	// immediately as soon as a call to ReaderMessage.Chunk returns false (note: Stream.RecvMsg may also return io.EOF).
	// This implementation may still be useful in such cases, see also the Buffered method.
	Reader struct {
		// Stream is the gRPC stream (client or server side - both are supported).
		Stream interface{ RecvMsg(m interface{}) error }

		// Factory is an implementation to support the stream's receive type.
		Factory ReaderMessageFactory

		// msg is the ReaderMessage.Value return value for Buffered, stored on message-based EOF
		msg atomic.Value
		// mu synchronises reads (it's not used for msg to avoid deadlocks)
		mu sync.Mutex
		// eof will be set when a ReaderMessage.Chunk returns false
		eof bool
		// msg is for "peeking" Stream.RecvMsg like recv(true), and is exposed via Buffered
		// buffer is from the last (data) message, if any
		buffer []byte
	}

	// ReaderMessage is part of an implementation for use with Reader. Each ReaderMessage value should wrap
	// a single underlying proto.Message (Value), and logic to extract data for the binary stream (Chunk).
	ReaderMessage interface {
		// Value should return a single proto.Message, to be passed into Reader.Stream.RecvMsg.
		Value() interface{}

		// Chunk should extract the stream chunk from Value, or return false, indicating EOF for the Reader.
		Chunk() ([]byte, bool)
	}

	// ReaderMessageFactory initialises a new ReaderMessage for use by Reader.
	ReaderMessageFactory func() ReaderMessage

	readerMessage struct {
		value interface{}
		chunk func() ([]byte, bool)
	}
)

// NewReaderMessageFactory constructs a new ReaderMessageFactory using closures as a quick and dirty way to implement
// the interface. Note that it's entirely optional, and only provided as a convenience.
func NewReaderMessageFactory(fn func() (value interface{}, chunk func() ([]byte, bool))) ReaderMessageFactory {
	if fn == nil {
		panic(`sesame/grpc: nil fn for reader message factory`)
	}
	return func() ReaderMessage {
		var m readerMessage
		m.value, m.chunk = fn()
		if m.value == nil || m.chunk == nil {
			panic(`sesame/grpc: reader message value or chunk was nil`)
		}
		return &m
	}
}

// Buffered returns any buffered (unread) message from the Reader, e.g. a message that caused
// ReaderMessage.Chunk to return false. The return value will be the result of ReaderMessage.Value, or nil.
func (x *Reader) Buffered() interface{} { return x.msg.Load() }

// Read implements io.Reader using Stream.
func (x *Reader) Read(b []byte) (int, error) {
	x.mu.Lock()
	defer x.mu.Unlock()

	if x.eof {
		return 0, io.EOF
	}

	if len(x.buffer) == 0 {
		x.buffer = nil

		msg, err := x.recv()
		if err != nil {
			return 0, err
		}

		chunk, ok := msg.Chunk()
		if !ok {
			x.eof = true
			x.msg.Store(msg.Value())
			return 0, io.EOF
		}

		x.buffer = chunk
	}

	n := copy(b, x.buffer)
	x.buffer = x.buffer[n:]
	return n, nil
}

func (x *Reader) recv() (msg ReaderMessage, err error) {
	msg = x.Factory()
	err = x.Stream.RecvMsg(msg.Value())
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (x *readerMessage) Value() interface{} { return x.value }

func (x *readerMessage) Chunk() ([]byte, bool) { return x.chunk() }
