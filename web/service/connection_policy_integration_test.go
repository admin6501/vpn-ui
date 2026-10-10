//go:build linux

package service

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/util/json_util"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// Exercise the bundled core on loopback, shortening the operator-configurable
// idle timeout to one second so a real socket-lifetime regression is practical.
func TestConnectionIdleClosesUDPAndKeepsActiveTCP(t *testing.T) {
	binary, err := filepath.Abs("../../corebundle/core/amd64/xray")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary); err != nil {
		t.Skip("bundled amd64 Xray not built")
	}
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	port := target.Addr().(*net.TCPAddr).Port
	udpTarget, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer udpTarget.Close()
	go func() {
		conn, err := target.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(8 * time.Second))
		data := make([]byte, 1)
		for {
			if _, err := conn.Read(data); err != nil {
				return
			}
			if _, err := conn.Write(data); err != nil {
				return
			}
		}

	}()
	udpRelayPort := make(chan int, 1)
	go func() {
		buffer := make([]byte, 256)
		first := true
		for {
			n, addr, err := udpTarget.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if first {
				udpRelayPort <- addr.Port
				first = false
			}
			udpTarget.WriteToUDP(buffer[:n], addr)
		}

	}()
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inboundPort := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()
	cfg := &xray.Config{Policy: json_util.RawMessage(`{"levels":{"0":{"connIdle":1}}}`), InboundConfigs: []xray.InboundConfig{{Tag: "idle-test", Protocol: "dokodemo-door", Listen: json_util.RawMessage(`"127.0.0.1"`), Port: inboundPort, Settings: json_util.RawMessage(fmt.Sprintf(`{"address":"127.0.0.1","port":%d,"network":"tcp,udp"}`, port))}}, OutboundConfigs: json_util.RawMessage(`[{"tag":"direct","protocol":"freedom"}]`)}
	if err := applyConnectionDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(t.TempDir(), "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command(binary, "run", "-config", path)
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	var tcp net.Conn
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		tcp, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", inboundPort), 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		logs, _ := os.ReadFile(log.Name())
		t.Fatalf("core start: %v\n%s", err, logs)
	}
	defer tcp.Close()
	tcp.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := tcp.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 1)
	if _, err := tcp.Read(reply); err != nil || reply[0] != 'x' {
		t.Fatalf("TCP relay: %v", err)
	}
	udp, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: inboundPort})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	udp.SetDeadline(time.Now().Add(4 * time.Second))
	udp.Write([]byte("u"))
	if _, err := udp.Read(reply); err != nil || reply[0] != 'u' {
		t.Fatalf("UDP relay: %v", err)
	}
	relayPort := <-udpRelayPort
	if !coreHasUDPLocal(cmd.Process.Pid, relayPort) {
		t.Fatal("UDP outbound socket was not observed before idle expiry")
	}
	// Traffic spanning several idle periods must keep both sessions alive.
	activeUntil := time.Now().Add(3 * time.Second)
	for time.Now().Before(activeUntil) {
		tcp.Write([]byte("x"))
		if _, err := tcp.Read(reply); err != nil {
			t.Fatalf("active TCP was closed: %v", err)
		}
		udp.Write([]byte("u"))
		if _, err := udp.Read(reply); err != nil {
			t.Fatalf("active UDP was closed: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	deadline = time.Now().Add(5 * time.Second)
	for coreHasUDPLocal(cmd.Process.Pid, relayPort) {
		if time.Now().After(deadline) {
			t.Fatal("idle UDP outbound socket remained open")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func coreHasUDPLocal(pid, port int) bool {
	owned := map[string]bool{}
	entries, _ := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	for _, entry := range entries {
		link, _ := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, entry.Name()))
		if strings.HasPrefix(link, "socket:[") {
			owned[strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")] = true
		}
	}
	for _, table := range []string{"udp", "udp6"} {
		data, _ := os.ReadFile("/proc/net/" + table)
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 9 && strings.HasSuffix(fields[1], fmt.Sprintf(":%04X", port)) && owned[fields[9]] {
				return true
			}
		}
	}
	return false
}

// Fill the target's receive window without reading it. Session idle is long,
// so only TCP_USER_TIMEOUT can release the resulting queued outbound promptly.
func TestConnectionQueuedTCPExpires(t *testing.T) {
	binary, err := filepath.Abs("../../corebundle/core/amd64/xray")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(binary); err != nil {
		t.Skip("bundled amd64 Xray not built")
	}
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	raw, err := listener.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var socketErr error
	if err := raw.Control(func(fd uintptr) {
		socketErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 4096)
	}); err != nil {
		t.Fatal(err)
	}
	if socketErr != nil {
		t.Fatal(socketErr)
	}
	stopped := make(chan struct{})
	defer close(stopped)
	accepted := make(chan struct{})
	go func() {
		target, err := listener.AcceptTCP()
		if err != nil {
			return
		}
		defer target.Close()
		close(accepted)
		<-stopped
	}()
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()
	cfg := &xray.Config{Policy: json_util.RawMessage(`{"levels":{"0":{"connIdle":60}}}`), InboundConfigs: []xray.InboundConfig{{Tag: "queue-test", Protocol: "dokodemo-door", Listen: json_util.RawMessage(`"127.0.0.1"`), Port: port, Settings: json_util.RawMessage(fmt.Sprintf(`{"address":"127.0.0.1","port":%d,"network":"tcp"}`, listener.Addr().(*net.TCPAddr).Port))}}, OutboundConfigs: json_util.RawMessage(`[{"protocol":"freedom","streamSettings":{"sockopt":{"tcpUserTimeout":1000}}}]`)}
	if err := applyConnectionDefaults(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(t.TempDir(), "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command(binary, "run", "-config", path)
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	var client net.Conn
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		client, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.SetDeadline(time.Now().Add(6 * time.Second))
	written := make(chan struct{})
	go func() { defer close(written); client.Write(make([]byte, 16*1024*1024)) }()
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("target never connected")
	}
	start := time.Now()
	var reply [1]byte
	_, err = client.Read(reply[:])
	if err == nil {
		t.Fatal("unexpected target data")
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("queued TCP survived the configured user timeout")
	}
	if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
		logs, _ := os.ReadFile(log.Name())
		t.Fatalf("connection failed before timeout: %v; %s", err, logs)
	}
	client.Close()
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("blocked client writer was not released")
	}
}
