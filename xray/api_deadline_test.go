package xray

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/proxyman/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stalledHandler struct {
	command.UnimplementedHandlerServiceServer
}

func (stalledHandler) AddInbound(ctx context.Context, _ *command.AddInboundRequest) (*command.AddInboundResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (stalledHandler) RemoveInbound(ctx context.Context, _ *command.RemoveInboundRequest) (*command.RemoveInboundResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (stalledHandler) AlterInbound(ctx context.Context, _ *command.AlterInboundRequest) (*command.AlterInboundResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// A core may accept TCP/gRPC connections but stop answering. Mutating the panel
// must fall back to the existing restart path instead of waiting indefinitely.
func TestHandlerCallsBoundStalledCore(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	command.RegisterHandlerServiceServer(server, stalledHandler{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	for _, tc := range []struct {
		name string
		call func(*XrayAPI) error
	}{
		{"add inbound", func(x *XrayAPI) error {
			return x.AddInbound([]byte(`{"tag":"t","protocol":"dokodemo-door","port":12345,"settings":{"address":"127.0.0.1","port":80,"network":"tcp"}}`))
		}},
		{"delete inbound", func(x *XrayAPI) error { return x.DelInbound("t") }},
		{"add user", func(x *XrayAPI) error {
			return x.AddUser("vmess", "t", map[string]any{"email": "e", "id": "11111111-2222-3333-4444-555555555555"})
		}},
		{"remove user", func(x *XrayAPI) error { return x.RemoveUser("t", "e") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api := new(XrayAPI)
			if err := api.Init(listener.Addr().(*net.TCPAddr).Port); err != nil {
				t.Fatal(err)
			}
			defer api.Close()
			started := time.Now()
			err := tc.call(api)
			// RemoveUser wraps the status error, so use FromError's wrapping support.
			if status.Code(err) != codes.DeadlineExceeded {
				t.Fatalf("want deadline error from stalled handler, got %v", err)
			}
			if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 8*time.Second {
				t.Fatalf("unexpected timeout: %s", elapsed)
			}
		})
	}
}
