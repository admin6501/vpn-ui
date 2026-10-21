package xray

import (
	"context"
	"testing"

	"github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type captureShadowsocksHandler struct {
	command.HandlerServiceClient
	request *command.AlterInboundRequest
}

func (h *captureShadowsocksHandler) AlterInbound(_ context.Context, request *command.AlterInboundRequest, _ ...grpc.CallOption) (*command.AlterInboundResponse, error) {
	h.request = request
	return &command.AlterInboundResponse{}, nil
}

func TestShadowsocksHotAddCompatibility(t *testing.T) {
	for _, test := range []struct {
		cipher string
		legacy shadowsocks.CipherType
	}{
		{"chacha20-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305},
		{"chacha20-ietf-poly1305", shadowsocks.CipherType_CHACHA20_POLY1305},
		{"xchacha20-poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305},
		{"xchacha20-ietf-poly1305", shadowsocks.CipherType_XCHACHA20_POLY1305},
		{"2022-blake3-aes-128-gcm", shadowsocks.CipherType_UNKNOWN},
		{"2022-blake3-aes-256-gcm", shadowsocks.CipherType_UNKNOWN},
	} {
		t.Run(test.cipher, func(t *testing.T) {
			handler := &captureShadowsocksHandler{}
			var client command.HandlerServiceClient = handler
			api := &XrayAPI{HandlerServiceClient: &client}
			if err := api.AddUser("shadowsocks", "inbound-test", map[string]any{"email": "Customer", "password": "secret", "cipher": test.cipher}); err != nil {
				t.Fatal(err)
			}
			op := new(command.AddUserOperation)
			if err := proto.Unmarshal(handler.request.Operation.Value, op); err != nil {
				t.Fatal(err)
			}
			if op.User.Email != "Customer" || handler.request.Tag != "inbound-test" {
				t.Fatal("API update lost account identity or inbound")
			}
			if test.legacy != shadowsocks.CipherType_UNKNOWN {
				if op.User.Account.Type != "xray.proxy.shadowsocks.Account" {
					t.Fatal("classic cipher used wrong wire account", op.User.Account.Type)
				}
				account := new(shadowsocks.Account)
				if err := proto.Unmarshal(op.User.Account.Value, account); err != nil {
					t.Fatal(err)
				}
				if account.CipherType != test.legacy || account.Password != "secret" {
					t.Fatal("classic account changed", account)
				}
			} else {
				if op.User.Account.Type != "xray.proxy.shadowsocks_2022.ServerConfig" {
					t.Fatal("2022 cipher used wrong wire account", op.User.Account.Type)
				}
				account := new(shadowsocks_2022.ServerConfig)
				if err := proto.Unmarshal(op.User.Account.Value, account); err != nil {
					t.Fatal(err)
				}
				if account.Key != "secret" || account.Email != "Customer" {
					t.Fatal("2022 credentials changed", account)
				}
			}
		})
	}
}
