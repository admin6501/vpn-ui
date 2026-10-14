package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func redactRepresentativeInboundSecrets(row *model.Inbound) {
	var settings map[string]json.RawMessage
	if json.Unmarshal([]byte(row.Settings), &settings) == nil {
		for _, key := range []string{"privateKey", "secretKey", "privateKeyFile", "keyFile", "serverKey", "serverKeyFile", "serverPrivateKey", "hostKey", "hostKeys", "tlsCryptKey", "caKey"} {
			delete(settings, key)
		}
		if body, err := json.Marshal(settings); err == nil {
			row.Settings = string(body)
		}
	} else {
		row.Settings = `{"clients":[]}`
	}
	var stream map[string]json.RawMessage
	if json.Unmarshal([]byte(row.StreamSettings), &stream) == nil {
		for _, key := range []string{"tlsSettings", "realitySettings"} {
			var value map[string]json.RawMessage
			if json.Unmarshal(stream[key], &value) == nil && value != nil {
				delete(value, "privateKey")
				delete(value, "mldsa65Seed")
				delete(value, "certificates")
				if body, err := json.Marshal(value); err == nil {
					stream[key] = body
				}
			}
		}
		if body, err := json.Marshal(stream); err == nil {
			row.StreamSettings = string(body)
		}
	} else {
		row.StreamSettings = `{}`
	}
}
