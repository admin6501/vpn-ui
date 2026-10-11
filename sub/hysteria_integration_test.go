//go:build linux

package sub

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestHysteriaRealCoreTCPAndUDP(t *testing.T) {
	binary := os.Getenv("HYSTERIA_TEST_CORE")
	if binary == "" {
		binary = filepath.Join("..", "corebundle", "core", runtime.GOARCH, "xray")
	}
	binary, _ = filepath.Abs(binary)
	if _, err := os.Stat(binary); err != nil {
		t.Skip("build embedded Xray before integration test")
	}
	for _, obfs := range []bool{false, true} {
		t.Run(map[bool]string{false: "TLS", true: "TLS-Salamander"}[obfs], func(t *testing.T) {
			dir := t.TempDir()
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
			der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			keyDer, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDer}), 0600); err != nil {
				t.Fatal(err)
			}
			tcp, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer tcp.Close()
			go func() {
				for {
					c, e := tcp.Accept()
					if e != nil {
						return
					}
					go func() { defer c.Close(); io.Copy(c, c) }()
				}
			}()
			udp, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer udp.Close()
			go func() {
				b := make([]byte, 65536)
				for {
					n, a, e := udp.ReadFrom(b)
					if e != nil {
						return
					}
					udp.WriteTo(b[:n], a)
				}
			}()
			reserve := func(network string) int {
				if network == "tcp" {
					l, e := net.Listen("tcp", "127.0.0.1:0")
					if e != nil {
						t.Fatal(e)
					}
					p := l.Addr().(*net.TCPAddr).Port
					l.Close()
					return p
				}
				l, e := net.ListenPacket("udp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				p := l.LocalAddr().(*net.UDPAddr).Port
				l.Close()
				return p
			}
			serverPort, tcpPort, udpPort := reserve("udp"), reserve("tcp"), reserve("udp")
			stream := map[string]any{"network": "hysteria", "security": "tls", "hysteriaSettings": map[string]any{"version": 2}, "tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": certPath, "keyFile": keyPath}}, "settings": map[string]any{"allowInsecure": false}}}
			certHash := sha256.Sum256(der)
			stream["tlsSettings"].(map[string]any)["pinnedPeerCertSha256"] = hex.EncodeToString(certHash[:])
			if obfs {
				stream["finalmask"] = map[string]any{"udp": []any{map[string]any{"type": "salamander", "settings": map[string]any{"password": "test-obfs"}}}}
			}
			const token = "test@auth:/?#%"
			server := map[string]any{"log": map[string]any{"loglevel": "debug"}, "inbounds": []any{map[string]any{"listen": "127.0.0.1", "port": serverPort, "protocol": "hysteria", "settings": map[string]any{"version": 2, "clients": []any{map[string]any{"email": "test-user", "auth": token}}}, "streamSettings": stream}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"ipsBlocked": []string{}}}}}
			rawStream, _ := json.Marshal(stream)
			in := &model.Inbound{Port: serverPort, Protocol: model.Hysteria, Settings: `{"version":2}`, StreamSettings: string(rawStream)}
			s := &SubJsonService{SubService: &SubService{remarkModel: "-ieo"}}
			configs := s.getConfig(in, model.Client{Email: "test-user", Auth: token}, "127.0.0.1")
			if len(configs) != 1 {
				t.Fatal("subscription profile missing")
			}
			var client map[string]any
			if err := json.Unmarshal(configs[0], &client); err != nil {
				t.Fatal(err)
			}
			client["log"] = map[string]any{"loglevel": "debug"}
			client["inbounds"] = []any{map[string]any{"listen": "127.0.0.1", "port": tcpPort, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1", "port": tcp.Addr().(*net.TCPAddr).Port, "network": "tcp"}}, map[string]any{"listen": "127.0.0.1", "port": udpPort, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1", "port": udp.LocalAddr().(*net.UDPAddr).Port, "network": "udp"}}}
			start := func(name string, cfg any) {
				raw, e := json.Marshal(cfg)
				if e != nil {
					t.Fatal(e)
				}
				p := filepath.Join(dir, name+".json")
				if e = os.WriteFile(p, raw, 0600); e != nil {
					t.Fatal(e)
				}
				log, e := os.Create(filepath.Join(dir, name+".log"))
				if e != nil {
					t.Fatal(e)
				}
				cmd := exec.Command(binary, "run", "-config", p)
				cmd.Stdout = log
				cmd.Stderr = log
				if e = cmd.Start(); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					cmd.Process.Kill()
					cmd.Wait()
					log.Close()
					if t.Failed() {
						b, _ := os.ReadFile(log.Name())
						t.Logf("%s: %s", name, b)
					}
				})
			}
			start("server", server)
			start("client", client)
			var c net.Conn
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				c, err = net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpPort)), 200*time.Millisecond)
				if err == nil {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(10 * time.Second))
			payload := bytes.Repeat([]byte("Hysteria TCP payload"), 512)
			if _, err = c.Write(payload); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, len(payload))
			if _, err = io.ReadFull(c, reply); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, reply) {
				t.Fatal("TCP payload changed")
			}
			u, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(udpPort)))
			if err != nil {
				t.Fatal(err)
			}
			defer u.Close()
			u.SetDeadline(time.Now().Add(10 * time.Second))
			if _, err = u.Write([]byte("Hysteria UDP payload")); err != nil {
				t.Fatal(err)
			}
			b := make([]byte, 100)
			n, err := u.Read(b)
			if err != nil {
				t.Fatal(err)
			}
			if string(b[:n]) != "Hysteria UDP payload" {
				t.Fatal("UDP payload changed")
			}
		})
	}
}
