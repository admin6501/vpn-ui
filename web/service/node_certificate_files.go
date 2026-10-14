package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"golang.org/x/sys/unix"
)

const maxNodeCertificateBytes = 4 << 20

type certificateReference struct {
	object            map[string]any
	field, kind, path string
}
type certificateDocument struct {
	target *string
	object map[string]any
}

// Only actual, active protocol certificate fields can transfer files. Never
// recursively search arbitrary JSON keys: those names are not authorization.
func inboundCertificateReferences(in *model.Inbound) ([]certificateDocument, []certificateReference, error) {
	var docs []certificateDocument
	var refs []certificateReference
	parse := func(target *string) (map[string]any, error) {
		obj := map[string]any{}
		if *target != "" {
			if err := json.Unmarshal([]byte(*target), &obj); err != nil {
				return nil, err
			}
		}
		docs = append(docs, certificateDocument{target, obj})
		return obj, nil
	}
	add := func(obj map[string]any, field, kind string) error {
		item, exists := obj[field]
		if !exists || item == nil {
			return nil
		}
		path, ok := item.(string)
		if !ok {
			return fmt.Errorf("certificate file path must be a string")
		}
		if path == "" {
			return nil
		}
		if strings.ContainsFunc(path, unicode.IsControl) {
			return fmt.Errorf("certificate file path contains control characters")
		}
		refs = append(refs, certificateReference{obj, field, kind, path})
		return nil
	}
	if in.Protocol == model.OPENVPN || in.Protocol == model.OPENCONNECT || in.Protocol == model.SSTP || in.Protocol == model.IKEV2 {
		obj, err := parse(&in.Settings)
		if err != nil {
			return nil, nil, err
		}
		fileMode, _ := obj["tlsUseFile"].(bool)
		if fileMode {
			fields := map[string]string{"certificateFile": "cert", "keyFile": "key"}
			if in.Protocol == model.OPENVPN {
				fields = map[string]string{"caCertFile": "cert", "serverCertFile": "cert", "serverKeyFile": "key"}
				if enabled, ok := obj["tlsCryptEnable"].(bool); !ok || enabled {
					fields["tlsCryptFile"] = "static"
				}
			}
			for field, kind := range fields {
				if err := add(obj, field, kind); err != nil {
					return nil, nil, err
				}
			}
		}
	} else if NodeCoreForProtocol(in.Protocol) == "xray" {
		obj, err := parse(&in.StreamSettings)
		if err != nil {
			return nil, nil, err
		}
		if security, _ := obj["security"].(string); security == "tls" {
			tlsSettings, _ := obj["tlsSettings"].(map[string]any)
			certs, _ := tlsSettings["certificates"].([]any)
			for _, item := range certs {
				cert, _ := item.(map[string]any)
				if err := add(cert, "certificateFile", "cert"); err != nil {
					return nil, nil, err
				}
				if err := add(cert, "keyFile", "key"); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return docs, refs, nil
}

// Delegated inbound managers can retain an existing certificate binding or pick
// a managed certificate. Only the panel owner may introduce another host path.
// This also covers local inbounds: moving one to a node must not grant file access.
func ValidateInboundCertificatePaths(user *model.User, before, after *model.Inbound) error {
	_, refs, err := inboundCertificateReferences(after)
	if err != nil {
		return err
	}
	if len(refs) == 0 || (user != nil && user.IsPanelOwner) {
		return nil
	}
	allowed := map[string]bool{}
	if before != nil {
		_, existing, e := inboundCertificateReferences(before)
		if e != nil {
			return e
		}
		for _, ref := range existing {
			allowed[ref.kind+":"+ref.path] = true
		}
	}
	for _, choice := range sslCertificateChoices() {
		allowed["cert:"+choice.CertPath] = true
		allowed["key:"+choice.KeyPath] = true
	}
	for _, ref := range refs {
		if !allowed[ref.kind+":"+ref.path] {
			return fmt.Errorf("Only the panel owner may set a new external certificate file path; select a managed certificate instead")
		}
	}
	return nil
}

func readNodeCertificate(path, kind string) ([]byte, error) {
	// Nonblocking open avoids a FIFO/device hanging the panel. Check the opened
	// descriptor, allowing the managed active symlink used during certificate renewals.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("certificate file unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("certificate must be a regular file")
	}
	if info.Size() > maxNodeCertificateBytes {
		return nil, fmt.Errorf("certificate file too large")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxNodeCertificateBytes+1))
	if err != nil || len(data) > maxNodeCertificateBytes {
		return nil, fmt.Errorf("certificate file unreadable or too large")
	}
	if err := validateNodeCertificateMaterial(data, kind); err != nil {
		return nil, err
	}
	return data, nil
}

func validateNodeCertificateMaterial(data []byte, kind string) error {
	if kind == "static" {
		// OpenVPN static keys are hexadecimal, not base64 PEM. Permit only the
		// generator's optional comment lines and a complete 2048-bit key block.
		var text []string
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				text = append(text, line)
			}
		}
		if len(text) < 3 || text[0] != "-----BEGIN OpenVPN Static key V1-----" || text[len(text)-1] != "-----END OpenVPN Static key V1-----" {
			return fmt.Errorf("invalid OpenVPN static key file")
		}
		decoded, err := hex.DecodeString(strings.Join(text[1:len(text)-1], ""))
		if err != nil || len(decoded) != 256 {
			return fmt.Errorf("invalid OpenVPN static key file")
		}
		return nil
	}
	rest := bytes.TrimSpace(data)
	count := 0
	for len(rest) > 0 {
		if !bytes.HasPrefix(rest, []byte("-----BEGIN ")) {
			return fmt.Errorf("file contains non-certificate material")
		}
		// Decode exactly the FIRST block. Decoding an entire file can skip a
		// malformed prefix and transfer it with a later valid certificate.
		lineEnd := bytes.IndexByte(rest, '\n')
		if lineEnd < 0 {
			return fmt.Errorf("invalid certificate PEM")
		}
		header := strings.TrimSuffix(string(rest[:lineEnd]), "\r")
		typeName := strings.TrimSuffix(strings.TrimPrefix(header, "-----BEGIN "), "-----")
		endMarker := []byte("-----END " + typeName + "-----")
		end := bytes.Index(rest, endMarker)
		if end < 0 {
			return fmt.Errorf("invalid certificate PEM")
		}
		end += len(endMarker)
		block, trailing := pem.Decode(rest[:end])
		if block == nil || len(block.Headers) != 0 || len(bytes.TrimSpace(trailing)) != 0 {
			return fmt.Errorf("invalid certificate PEM")
		}
		tail := rest[end:]
		if kind == "cert" {
			if block.Type != "CERTIFICATE" {
				return fmt.Errorf("expected a certificate file")
			}
			if _, err := x509.ParseCertificate(block.Bytes); err != nil {
				return fmt.Errorf("invalid certificate")
			}
		} else {
			var err error
			switch block.Type {
			case "PRIVATE KEY":
				_, err = x509.ParsePKCS8PrivateKey(block.Bytes)
			case "RSA PRIVATE KEY":
				_, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			case "EC PRIVATE KEY":
				_, err = x509.ParseECPrivateKey(block.Bytes)
			default:
				return fmt.Errorf("expected a TLS private key file")
			}
			if err != nil {
				return fmt.Errorf("invalid TLS private key")
			}
		}
		count++
		rest = bytes.TrimSpace(tail)
	}
	if count == 0 || (kind == "key" && count != 1) {
		return fmt.Errorf("empty or ambiguous certificate material")
	}
	return nil
}

func snapshotCertificateFiles(inbound *model.Inbound, files map[string][]byte) error {
	docs, refs, err := inboundCertificateReferences(inbound)
	if err != nil {
		return err
	}
	// Stage everything before mutating the snapshot. A bad path leaves both the
	// caller's JSON and file map unchanged, rather than publishing half a bundle.
	pending := map[string][]byte{}
	names := make([]string, len(refs))
	for i, ref := range refs {
		data, err := readNodeCertificate(ref.path, ref.kind)
		if err != nil {
			return fmt.Errorf("Inbound #%d: %w", inbound.Id, err)
		}
		sum := sha256.Sum256(data)
		names[i] = hex.EncodeToString(sum[:])
		pending[names[i]] = data
	}
	for i, ref := range refs {
		ref.object[ref.field] = "nodefile:" + names[i]
	}
	for _, doc := range docs {
		if len(refs) == 0 {
			continue
		}
		body, err := json.Marshal(doc.object)
		if err != nil {
			return err
		}
		*doc.target = string(body)
	}
	for name, data := range pending {
		files[name] = data
	}
	return nil
}
