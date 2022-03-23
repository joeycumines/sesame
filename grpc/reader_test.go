package grpc_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	grpcutil "github.com/joeycumines/sesame/grpc"
	"github.com/joeycumines/sesame/internal/testutil"
	"github.com/joeycumines/sesame/rc"
	streamutil "github.com/joeycumines/sesame/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// ExampleNewReaderMessageFactory demonstrates how to implement ReaderMessageFactory using NewReaderMessageFactory.
func ExampleNewReaderMessageFactory() {
	// construct a ReaderMessageFactory to handle sesame.v1alpha1.NetConnResponse messages
	factory := grpcutil.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
		var msg rc.NetConnResponse
		value = &msg
		chunk = func() ([]byte, bool) {
			if v, ok := msg.GetData().(*rc.NetConnResponse_Bytes); ok {
				return v.Bytes, true
			}
			return nil, false
		}
		return
	})

	// factory would normally be used as a grpc.Reader's Factory field, but just to demonstrate the behavior...

	unmarshal := func(s string) grpcutil.ReaderMessage {
		msg := factory()
		if err := protojson.Unmarshal([]byte(s), msg.Value().(proto.Message)); err != nil {
			panic(err)
		}
		return msg
	}

	p := func(s string) {
		msg := unmarshal(s)
		b, ok := msg.Chunk()
		fmt.Printf("\nmsg %s;\n-> %v;\n-> ok=%v chunk=%q;\n", s, msg.Value(), ok, b)
	}

	fmt.Println(`using factory impl. for sesame.v1alpha1.NetConnResponse...`)
	p(`{"bytes":"c29tZSBkYXRh"}`)

	p(`{"bytes":""}`)

	p(`{"bytes":"MTIz"}`)

	p(`{"conn":{}}`)

	p(`{}`)

	//output:
	//using factory impl. for sesame.v1alpha1.NetConnResponse...
	//
	//msg {"bytes":"c29tZSBkYXRh"};
	//-> bytes:"some data";
	//-> ok=true chunk="some data";
	//
	//msg {"bytes":""};
	//-> bytes:"";
	//-> ok=true chunk="";
	//
	//msg {"bytes":"MTIz"};
	//-> bytes:"123";
	//-> ok=true chunk="123";
	//
	//msg {"conn":{}};
	//-> conn:{};
	//-> ok=false chunk="";
	//
	//msg {};
	//-> ;
	//-> ok=false chunk="";
}

// ExampleReader demonstrates how to use Reader by partially implementing the NetConn method of rc.RemoteControlServer.
func ExampleReader() {
	defer testutil.CheckNumGoroutines(nil, runtime.NumGoroutine(), false, time.Second*15)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*3)
	defer cancel()

	// this will be our target, that the server will proxy to (via targetConn)
	targetConn, targetListener := net.Pipe()
	// this can be ignored - it's just mocking the behavior of the target conn
	// it implements ping-pong line buffered messages, hello world style
	go func() {
		defer targetListener.Close()
		scanner := bufio.NewScanner(targetListener)
		for ctx.Err() == nil {
			// poll (until cancel)
			if targetListener.SetReadDeadline(time.Now().Add(time.Second*5)) != nil {
				return
			}
			if !scanner.Scan() {
				if errors.Is(scanner.Err(), os.ErrDeadlineExceeded) {
					continue
				}
				return
			}
			_ = targetListener.SetWriteDeadline(time.Now().Add(time.Second * 15))
			if _, err := fmt.Fprintf(targetListener, "hello %s!\n", bytes.TrimSpace(scanner.Bytes())); err != nil {
				panic(err)
			}
		}
	}()

	// implement a simple proxy impl with no support for the proper conn flow
	out := make(chan error, 1)
	control := mockRemoteControlServer{netConn: func(stream rc.RemoteControl_NetConnServer) (err error) {
		defer func() { out <- err }()

		// pretend there was something useful here, that to initialised targetConn
		defer targetConn.Close()

		reader := grpcutil.Reader{Stream: stream, Factory: grpcutil.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
			var msg rc.NetConnRequest
			value = &msg
			chunk = func() ([]byte, bool) {
				if v, ok := msg.GetData().(*rc.NetConnRequest_Bytes); ok {
					return v.Bytes, true
				}
				return nil, false
			}
			return
		})}

		writer := streamutil.ChunkWriter(func(b []byte) (int, error) {
			if err := stream.Send(&rc.NetConnResponse{Data: &rc.NetConnResponse_Bytes{Bytes: b}}); err != nil {
				return 0, err
			}
			return len(b), nil
		})

		type (
			ioReader   io.Reader
			ioWriter   io.Writer
			readWriter struct {
				ioReader
				ioWriter
			}
		)

		err = streamutil.Proxy(ctx, readWriter{&reader, writer}, targetConn)
		if v := reader.Buffered(); v != nil {
			panic(v)
		}
		if err != nil {
			return
		}

		err = targetConn.Close()
		if err != nil {
			return
		}

		return
	}}

	// connect to the server
	conn := testutil.NewBufconnClient(0, func(_ *bufconn.Listener, srv *grpc.Server) { rc.RegisterRemoteControlServer(srv, &control) })
	defer conn.Close()
	stream, err := rc.NewRemoteControlClient(conn).NetConn(ctx)
	if err != nil {
		panic(err)
	}

	// communicates with targetListener over stream
	pingPong := func(name string) {
		fmt.Printf("send %q\n", name)
		if err := stream.Send(&rc.NetConnRequest{Data: &rc.NetConnRequest_Bytes{Bytes: append(append(make([]byte, 0, len(name)+1), name...), '\n')}}); err != nil {
			panic(err)
		}
		msg, err := stream.Recv()
		if err != nil {
			panic(err)
		}
		if !bytes.HasSuffix(msg.GetBytes(), []byte("\n")) {
			panic(fmt.Errorf(`unexpected message %q in message: %s`, msg.GetBytes(), msg))
		}
		fmt.Printf("recv %q\n", msg.GetBytes()[:len(msg.GetBytes())-1])
	}

	pingPong("one")
	pingPong("two")
	pingPong("three")
	pingPong("four")

	if err := stream.CloseSend(); err != nil {
		panic(err)
	}

	if msg, err := stream.Recv(); err != io.EOF {
		panic(fmt.Errorf(`unexpected receive: %v %v`, msg, err))
	}

	if err := <-out; err != nil {
		panic(err)
	}

	//output:
	//send "one"
	//recv "hello one!"
	//send "two"
	//recv "hello two!"
	//send "three"
	//recv "hello three!"
	//send "four"
	//recv "hello four!"
}

func TestReader_Buffered(t *testing.T) {
	// message type is *interface{}
	factory := grpcutil.NewReaderMessageFactory(func() (value interface{}, chunk func() ([]byte, bool)) {
		var msg interface{}
		value = &msg
		chunk = func() (b []byte, ok bool) {
			b, ok = msg.([]byte)
			return
		}
		return
	})

	messages := [...]interface{}{
		[]byte(`0`),
		[]byte(`1`),
		[]byte(`2`),
		[]byte(`3`),
		[]byte(`4`),
		5,
		[]byte(`i6`),
		[]byte(`i7`),
		8,
		9,
	}
	ch := make(chan interface{}, len(messages))
	for _, v := range messages {
		ch <- v
	}
	close(ch)

	stream := &mockStreamRecvMsg{recvMsg: func(m interface{}) error {
		v, ok := <-ch
		if !ok {
			return io.EOF
		}
		*m.(*interface{}) = v
		return nil
	}}

	reader := &grpcutil.Reader{Stream: stream, Factory: factory}
	if v := reader.Buffered(); v != nil {
		t.Fatal(v)
	}

	buf := make([]byte, 8)

	for i := 0; i < 5; i++ {
		n, err := reader.Read(buf)
		if err != nil || n != 1 {
			t.Fatal(n, err)
		}
		if len(ch) != len(messages)-i-1 {
			t.Fatal(i, len(ch))
		}
		if string(buf[:n]) != strconv.Itoa(i) {
			t.Fatal(i, string(buf[:n]))
		}
		if v := reader.Buffered(); v != nil {
			t.Fatal(v)
		}
	}

	for i := 0; i < 3; i++ {
		if n, err := reader.Read(buf); err != io.EOF || n != 0 {
			t.Fatal(n, err)
		}
		if len(ch) != len(messages)-6 {
			t.Fatal(len(ch))
		}
		if v := reader.Buffered(); (*v.(*interface{})).(int) != 5 {
			t.Fatal(v)
		}
	}

	reader = &grpcutil.Reader{Stream: stream, Factory: factory}

	for i := 6; i < 8; i++ {
		n, err := reader.Read(buf[:1])
		if err != nil || n != 1 {
			t.Fatal(n, err)
		}
		if len(ch) != len(messages)-i-1 {
			t.Fatal(i, len(ch))
		}
		if v := reader.Buffered(); v != nil {
			t.Fatal(v)
		}

		n, err = reader.Read(buf[1:])
		if err != nil || n != 1 {
			t.Fatal(n, err)
		}
		if len(ch) != len(messages)-i-1 {
			t.Fatal(i, len(ch))
		}
		if v := reader.Buffered(); v != nil {
			t.Fatal(v)
		}

		if string(buf[:2]) != `i`+strconv.Itoa(i) {
			t.Fatal(i, string(buf[:n]))
		}
	}

	for _, next := range [...]int{8, 9} {
		for i := 0; i < 3; i++ {
			if n, err := reader.Read(buf); err != io.EOF || n != 0 {
				t.Fatal(n, err)
			}
			if len(ch) != len(messages)-next-1 {
				t.Fatal(len(ch))
			}
			if v := reader.Buffered(); (*v.(*interface{})).(int) != next {
				t.Fatal(v)
			}
		}

		reader = &grpcutil.Reader{Stream: stream, Factory: factory}
	}

	for i := 0; i < 3; i++ {
		if n, err := reader.Read(buf); err != io.EOF || n != 0 {
			t.Fatal(n, err)
		}
		if len(ch) != 0 {
			t.Fatal(len(ch))
		}
		if v := reader.Buffered(); v != nil {
			t.Fatal(v)
		}
	}
}
