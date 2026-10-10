package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/util/json_util"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestConnectionDefaultsPreserveOperatorPolicy(t *testing.T) {
	config := &xray.Config{
		Policy:          json_util.RawMessage(`{"levels":{"7":{"connIdle":900,"uplinkOnly":0}},"system":{"statsInboundUplink":true}}`),
		InboundConfigs:  []xray.InboundConfig{{Tag: "test", Protocol: "vless", StreamSettings: json_util.RawMessage(`{"network":"ws","wsSettings":{"path":"/keep"},"sockopt":{"mark":123,"tcpUserTimeout":0}}`)}},
		OutboundConfigs: json_util.RawMessage(`[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"blocked"}]`),
	}
	if err := applyConnectionDefaults(config); err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	json.Unmarshal(config.Policy, &policy)
	levels := policy["levels"].(map[string]any)
	if levels["0"].(map[string]any)["connIdle"] != float64(120) {
		t.Fatal("missing idle timeout")
	}
	seven := levels["7"].(map[string]any)
	if seven["connIdle"] != float64(900) || seven["uplinkOnly"] != float64(0) {
		t.Fatal("custom timeouts overwritten")
	}
	var stream map[string]any
	json.Unmarshal(config.InboundConfigs[0].StreamSettings, &stream)
	socket := stream["sockopt"].(map[string]any)
	if socket["tcpUserTimeout"] != float64(0) || socket["mark"] != float64(123) || stream["wsSettings"].(map[string]any)["path"] != "/keep" {
		t.Fatal("custom transport overwritten")
	}
	var outbounds []map[string]any
	json.Unmarshal(config.OutboundConfigs, &outbounds)
	if outbounds[0]["streamSettings"].(map[string]any)["sockopt"].(map[string]any)["tcpUserTimeout"] != float64(120000) {
		t.Fatal("outbound timeout missing")
	}
	if _, ok := outbounds[1]["streamSettings"]; ok {
		t.Fatal("blackhole modified")
	}
	first, _ := json.Marshal(config)
	if err := applyConnectionDefaults(config); err != nil {
		t.Fatal(err)
	}
	second, _ := json.Marshal(config)
	if string(first) != string(second) {
		t.Fatal("config changes on every generation")
	}
}

func TestConnectionDefaultsRespectDisabledKeepalive(t *testing.T) {
	for _, key := range []string{"tcpKeepAliveIdle", "tcpKeepAliveInterval"} {
		data, err := connectionStreamDefaults(json_util.RawMessage(`{"sockopt":{"` + key + `":-1}}`))
		if err != nil {
			t.Fatal(err)
		}
		var stream map[string]any
		json.Unmarshal(data, &stream)
		socket := stream["sockopt"].(map[string]any)
		if socket["tcpKeepAliveIdle"].(float64)*socket["tcpKeepAliveInterval"].(float64) < 0 {
			t.Fatal("disabled keepalive became invalid")
		}
	}
	if _, err := connectionStreamDefaults(json_util.RawMessage(`{"sockopt":"bad"}`)); err == nil {
		t.Fatal("invalid socket config silently replaced")
	}
}
