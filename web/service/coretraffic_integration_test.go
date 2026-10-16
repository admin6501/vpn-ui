package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// Real VMess and Shadowsocks TCP/UDP flows share one account while QueryStats
// repeatedly resets counters. Both memberships must keep their own exact bytes,
// and the account must be charged their sum once. No internet/root is needed.
func TestCoreTrafficRealConcurrentProtocols(t *testing.T) {
	if os.Getenv("RUN_CORE_TRAFFIC_INTEGRATION") != "1" {
		t.Skip("enable RUN_CORE_TRAFFIC_INTEGRATION=1 with XRAY_TRAFFIC_BINARY")
	}
	binary := os.Getenv("XRAY_TRAFFIC_BINARY")
	if binary == "" {
		t.Fatal("XRAY_TRAFFIC_BINARY is required")
	}
	svc := newAccountsDB(t)
	const email = "real-traffic@example.com"
	const uuid = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	tcpEcho, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpEcho.Close()
	go func() {
		for {
			c, err := tcpEcho.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udpEcho, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpEcho.Close()
	go func() {
		b := make([]byte, 65535)
		for {
			n, addr, err := udpEcho.ReadFrom(b)
			if err != nil {
				return
			}
			udpEcho.WriteTo(b[:n], addr)
		}
	}()
	tcpTarget := tcpEcho.Addr().(*net.TCPAddr).Port
	udpTarget := udpEcho.LocalAddr().(*net.UDPAddr).Port
	port := func() int {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		n := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", n))
		if err != nil {
			t.Fatal(err)
		}
		u.Close()
		return n
	}
	vmessPort, ssPort, apiPort := port(), port(), port()
	vmess := seedTaggedInbound(t, model.VMESS, vmessPort, []map[string]any{{"id": uuid, "email": email, "enable": true, "totalGB": float64(0)}})
	ss := seedTaggedInbound(t, model.Shadowsocks, ssPort, nil)
	seedAccountOn(t, svc, email, vmess, ss)
	seedAccountTraffic(t, email, vmess.Id)
	inbound := func(tag, protocol string, p int, settings any) map[string]any {
		return map[string]any{"tag": tag, "protocol": protocol, "port": p, "listen": "127.0.0.1", "settings": settings}
	}
	server := map[string]any{
		"log": map[string]any{"loglevel": "warning"}, "stats": map[string]any{},
		"policy": map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true}}, "system": map[string]any{"statsInboundUplink": true, "statsInboundDownlink": true}},
		"api":    map[string]any{"tag": "api", "services": []string{"StatsService"}},
		"inbounds": []any{
			inbound(vmess.Tag, "vmess", vmessPort, map[string]any{"clients": []any{map[string]any{"id": uuid, "email": email}}}),
			inbound(ss.Tag, "shadowsocks", ssPort, map[string]any{"method": "chacha20-ietf-poly1305", "clients": []any{map[string]any{"method": "chacha20-ietf-poly1305", "password": "integration-secret", "email": email}}, "network": "tcp,udp"}),
			inbound("api", "dokodemo-door", apiPort, map[string]any{"address": "127.0.0.1"}),
		},
		"outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}},
		"routing":   map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}},
	}
	entry := []int{port(), port(), port(), port()}
	var ins, rules []any
	for i, name := range []string{"vmess-tcp", "vmess-udp", "ss-tcp", "ss-udp"} {
		network := "tcp"
		target := tcpTarget
		if i%2 == 1 {
			network = "udp"
			target = udpTarget
		}
		out := "vmess-out"
		if i >= 2 {
			out = "ss-out"
		}
		ins = append(ins, inbound(name, "dokodemo-door", entry[i], map[string]any{"address": "127.0.0.1", "port": target, "network": network}))
		rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{name}, "outboundTag": out})
	}
	client := map[string]any{
		"log": map[string]any{"loglevel": "warning"}, "inbounds": ins, "routing": map[string]any{"rules": rules},
		"outbounds": []any{
			map[string]any{"tag": "vmess-out", "protocol": "vmess", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": vmessPort, "users": []any{map[string]any{"id": uuid, "security": "aes-128-gcm"}}}}}},
			map[string]any{"tag": "ss-out", "protocol": "shadowsocks", "settings": map[string]any{"servers": []any{map[string]any{"address": "127.0.0.1", "port": ssPort, "method": "chacha20-ietf-poly1305", "password": "integration-secret"}}}},
		},
	}
	start := func(name string, conf any, ready int) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		data, err := json.Marshal(conf)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		log, err := os.Create(filepath.Join(dir, "core.log"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, binary, "run", "-config", path)
		cmd.Stdout = log
		cmd.Stderr = log
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("core did not stop")
			}
			log.Close()
			if t.Failed() {
				b, _ := os.ReadFile(log.Name())
				t.Logf("%s core:\n%s", name, b)
			}
		})
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ready), 100*time.Millisecond)
			if err == nil {
				c.Close()
				return
			}
			select {
			case err := <-done:
				done <- err
				t.Fatalf("%s core stopped: %v", name, err)
			default:
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s core did not listen", name)
	}
	start("server", server, apiPort)
	start("client", client, entry[0])
	var api xray.XrayAPI
	if err = api.Init(apiPort); err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i, p := range entry {
		wg.Add(1)
		go func(i, p int) {
			defer wg.Done()
			network := "tcp"
			size, count := 32768, 8
			if i >= 2 {
				size, count = 49152, 11
			}
			if i%2 == 1 {
				network = "udp"
				size, count = 1000, 5
				if i >= 2 {
					size = 1500
				}
			}
			c, err := net.DialTimeout(network, fmt.Sprintf("127.0.0.1:%d", p), 5*time.Second)
			if err != nil {
				errors <- err
				return
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(15 * time.Second))
			payload := bytes.Repeat([]byte{byte(i + 1)}, size)
			reply := make([]byte, size)
			for n := 0; n < count; n++ {
				if _, err = c.Write(payload); err != nil {
					errors <- fmt.Errorf("flow %d write: %w", i, err)
					return
				}
				if _, err = io.ReadFull(c, reply); err != nil {
					errors <- fmt.Errorf("flow %d read: %w", i, err)
					return
				}
				if !bytes.Equal(payload, reply) {
					errors <- fmt.Errorf("flow %d payload mismatch", i)
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}(i, p)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	inbounds := &InboundService{}
	collect := func() {
		t.Helper()
		tags, rows, err := api.GetTraffic(true)
		if err != nil {
			t.Fatal(err)
		}
		err, _, _, _, _ = inbounds.AddTraffic(tags, rows)
		if err != nil {
			t.Fatal(err)
		}
	}
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	running := true
	for running {
		select {
		case <-done:
			running = false
		case <-ticker.C:
			collect()
		}
	}
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if t.Failed() {
		return
	}
	// A final snapshot drains bytes written since the last mid-transfer reset.
	collect()
	expected := map[int]int64{vmess.Id: 32768*8 + 1000*5, ss.Id: 49152*11 + 1500*5}
	var sum int64
	for id, want := range expected {
		share := membershipUsageOf(t, email, id)
		if share.Up != want || share.Down != want {
			t.Errorf("inbound %d measured %d/%d, want %d/%d", id, share.Up, share.Down, want, want)
		}
		sum += 2 * want
		row := renderedStats(t, id, email)
		if row.SharedUp+row.SharedDown != 0 {
			t.Errorf("new exact traffic became unattributed: %+v", row)
		}
	}
	total := accountTrafficOf(t, email)
	if total.Up+total.Down != sum || total.AllTime != sum {
		t.Errorf("charged %d / allTime %d, want exactly %d", total.Up+total.Down, total.AllTime, sum)
	}
	collect()
	again := accountTrafficOf(t, email)
	if again.Up != total.Up || again.Down != total.Down {
		t.Fatal("empty collection tick charged the same transfer again")
	}
}
