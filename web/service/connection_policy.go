package service

import (
	"encoding/json"
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/util/json_util"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

// Supply finite lifetimes when an imported config omits them. Explicit operator
// values, including zero, are preserved. No host-wide sysctl is changed.
func applyConnectionDefaults(config *xray.Config) error {
	var policy map[string]any
	if len(config.Policy) > 0 {
		if err := json.Unmarshal(config.Policy, &policy); err != nil {
			return fmt.Errorf("connection policy: %w", err)
		}
	}
	if policy == nil {
		policy = map[string]any{}
	}
	levels, ok := policy["levels"].(map[string]any)
	if !ok {
		if policy["levels"] != nil {
			return fmt.Errorf("connection policy levels must be an object")
		}
		levels = map[string]any{}
		policy["levels"] = levels
	}
	if _, ok := levels["0"]; !ok {
		levels["0"] = map[string]any{}
	}
	for level, value := range levels {
		settings, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("connection policy level %s must be an object", level)
		}
		setMissing(settings, "connIdle", 120)
		setMissing(settings, "uplinkOnly", 1)
		setMissing(settings, "downlinkOnly", 1)
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	config.Policy = data
	for i := range config.InboundConfigs {
		inbound := &config.InboundConfigs[i]
		if inbound.Tag == "api" || inbound.Protocol == "wireguard" {
			continue
		}
		inbound.StreamSettings, err = connectionStreamDefaults(inbound.StreamSettings)
		if err != nil {
			return fmt.Errorf("inbound %s: %w", inbound.Tag, err)
		}
	}
	if len(config.OutboundConfigs) > 0 && string(config.OutboundConfigs) != "null" {
		var outbounds []map[string]any
		if err := json.Unmarshal(config.OutboundConfigs, &outbounds); err != nil {
			return err
		}
		for _, outbound := range outbounds {
			switch outbound["protocol"] {
			case "blackhole", "dns", "wireguard":
				continue
			}
			raw, err := json.Marshal(outbound["streamSettings"])
			if err != nil {
				return err
			}
			stream, err := connectionStreamDefaults(raw)
			if err != nil {
				return fmt.Errorf("outbound %v: %w", outbound["tag"], err)
			}
			var value map[string]any
			if err := json.Unmarshal(stream, &value); err != nil {
				return err
			}
			outbound["streamSettings"] = value
		}
		config.OutboundConfigs, err = json.Marshal(outbounds)
		if err != nil {
			return err
		}
	}
	return nil
}

func setMissing(values map[string]any, key string, value any) {
	if _, ok := values[key]; !ok {
		values[key] = value
	}
}

func connectionStreamDefaults(raw json_util.RawMessage) (json_util.RawMessage, error) {
	var stream map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &stream); err != nil {
			return nil, err
		}
	}
	if stream == nil {
		stream = map[string]any{}
	}
	socket, ok := stream["sockopt"].(map[string]any)
	if !ok {
		if stream["sockopt"] != nil {
			return nil, fmt.Errorf("sockopt must be an object")
		}
		socket = map[string]any{}
		stream["sockopt"] = socket
	}
	// Milliseconds: bounded unacknowledged data/zero-window stalls, rather than
	// treating a momentarily nonempty send queue as a dead connection.
	setMissing(socket, "tcpUserTimeout", 120000)
	idle, interval := 60, 15
	if value, ok := socket["tcpKeepAliveIdle"].(float64); ok && value < 0 {
		interval = -1
	}
	if value, ok := socket["tcpKeepAliveInterval"].(float64); ok && value < 0 {
		idle = -1
	}
	setMissing(socket, "tcpKeepAliveIdle", idle)
	setMissing(socket, "tcpKeepAliveInterval", interval)
	return json.Marshal(stream)
}
