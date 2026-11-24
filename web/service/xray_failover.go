package service

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/xray"
)

// The panel stores the primary separately for editing. The bundled core reads
// it as the first exact tag in the failover selector list.
func compileFailoverBalancers(config *xray.Config) error {
	if len(config.RouterConfig) == 0 {
		return nil
	}
	var routing map[string]json.RawMessage
	if err := json.Unmarshal(config.RouterConfig, &routing); err != nil {
		return err
	}
	var balancers []json.RawMessage
	if raw := routing["balancers"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &balancers); err != nil {
			return err
		}
	}
	type balancerConfig struct {
		Tag        string
		Selector   []string
		PrimaryTag string
		Strategy   struct{ Type string }
	}
	failovers := make(map[int]balancerConfig)
	for i, raw := range balancers {
		var balancer balancerConfig
		if err := json.Unmarshal(raw, &balancer); err != nil {
			return err
		}
		if strings.EqualFold(balancer.Strategy.Type, "failover") {
			failovers[i] = balancer
		}
	}
	if len(failovers) == 0 {
		return nil
	}

	var outbounds []json.RawMessage
	if err := json.Unmarshal(config.OutboundConfigs, &outbounds); err != nil {
		return err
	}
	tags := make(map[string]bool)
	for _, raw := range outbounds {
		var outbound struct {
			Tag string
		}
		if err := json.Unmarshal(raw, &outbound); err != nil {
			return err
		}
		tags[outbound.Tag] = true
	}
	// If every measured path is down, keep traffic from falling through
	// to the core's default outbound.
	block := "vpnui-failover-block"
	for tags[block] {
		block += "-"
	}
	marshal := func(value any) json.RawMessage {
		// Only JSON-compatible types and decoded RawMessages reach this helper.
		raw, _ := json.Marshal(value)
		return raw
	}
	outbounds = append(outbounds, marshal(map[string]string{"tag": block, "protocol": "blackhole"}))

	observatory := make(map[string]json.RawMessage)
	if len(config.Observatory) != 0 && string(config.Observatory) != "null" {
		if err := json.Unmarshal(config.Observatory, &observatory); err != nil {
			return err
		}
	}
	var probeTags []string
	if raw := observatory["subjectSelector"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &probeTags); err != nil {
			return err
		}
	}
	for i := range balancers {
		b, ok := failovers[i]
		if !ok {
			continue
		}
		if b.Tag == "" || b.PrimaryTag == "" || len(b.Selector) == 0 {
			return fmt.Errorf("failover balancer %q requires a primary and at least one backup outbound", b.Tag)
		}
		if slices.Contains(b.Selector, b.PrimaryTag) {
			return fmt.Errorf("failover balancer %q lists its primary as a backup", b.Tag)
		}
		members := append([]string{b.PrimaryTag}, b.Selector...)
		for _, tag := range members {
			if tag == "" || !tags[tag] {
				return fmt.Errorf("failover balancer %q has an unknown outbound %q", b.Tag, tag)
			}
			if !slices.Contains(probeTags, tag) {
				probeTags = append(probeTags, tag)
			}
		}
		balancers[i] = marshal(map[string]any{
			"tag": b.Tag, "selector": members, "fallbackTag": block,
			"strategy": map[string]string{"type": "failover"},
		})
	}
	routing["balancers"] = marshal(balancers)
	observatory["subjectSelector"] = marshal(probeTags)
	for key, value := range map[string]any{
		"probeInterval": "10s", "probeURL": "https://www.google.com/generate_204", "enableConcurrency": true,
	} {
		if len(observatory[key]) == 0 {
			observatory[key] = marshal(value)
		}
	}
	config.RouterConfig = []byte(marshal(routing))
	config.OutboundConfigs = []byte(marshal(outbounds))
	config.Observatory = []byte(marshal(observatory))
	return nil
}
