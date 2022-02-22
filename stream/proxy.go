package stream

import (
	"context"
	"io"
)

type (
	// copyOp models the result of an io.Copy operation, and is used by copyUnidirectional and copyBidirectional.
	copyOp struct {
		N   int64
		Err error
	}
)

// Proxy connects two bidirectional streams, where local should be the initiator of the connection.
// This implementation can support local-initiated partial close, in which case remote.Close should only close remote's
// writer (called on io.EOF from local's reader). If remote returns io.EOF, or copying fails, then remote.Close will
// be called, but the send operation (copying from local to remote) WILL NOT be waited for.
func Proxy(ctx context.Context, local io.ReadWriter, remote io.ReadWriteCloser) (err error) {
	send, receive := copyBidirectional(local, remote)
	_, _, err = waitCopy(ctx, send, receive, remote)
	return
}

// copyBidirectional performs bidirectional stream copying, using io.Copy.
func copyBidirectional(local, remote io.ReadWriter) (send, receive <-chan copyOp) {
	send = copyUnidirectional(remote, local)
	receive = copyUnidirectional(local, remote)
	return
}

// copyUnidirectional calls io.Copy, returning the result via a channel.
func copyUnidirectional(dst io.Writer, src io.Reader) <-chan copyOp {
	ch := make(chan copyOp, 1)
	go func() {
		op := copyOp{Err: ErrPanic}
		defer func() { ch <- op }()
		op.N, op.Err = io.Copy(dst, src)
	}()
	return ch
}

// waitCopy will block until receive is finished, calling the provided closer after either send or receive, always
// calling it exactly once prior to exit, unless a panic occurs, or context is canceled. The results (sent and received)
// will contain any values received from send or receive, respectively.
func waitCopy(ctx context.Context, send, receive <-chan copyOp, closer io.Closer) (sent, received *copyOp, err error) {
	select {
	case <-ctx.Done():
		err = ctx.Err()

	case sentVal := <-send:
		sent = &sentVal

		err = closer.Close()
		if sentVal.Err != nil {
			err = sentVal.Err
		}

		select {
		case <-ctx.Done():
			err = ctx.Err()
		case receivedVal := <-receive:
			received = &receivedVal
			// NOTE any receive error is ignored, in this case
		}

	case receivedVal := <-receive:
		received = &receivedVal

		err = closer.Close()
		if receivedVal.Err != nil {
			err = receivedVal.Err
		}
	}

	return
}
