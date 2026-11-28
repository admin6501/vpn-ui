package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/xray"
)

const gatewayStrategyName = "gateway"

type gatewaySettings struct {
	LatencyMS                int      `json:"latencyMs"`
	IntervalSeconds          int      `json:"intervalSeconds"`
	Attempts                 int      `json:"attempts"`
	EmergencyIntervalSeconds int      `json:"emergencyIntervalSeconds"`
	EmergencyAttempts        int      `json:"emergencyAttempts"`
	EmergencyTags            []string `json:"emergencyTags"`
	ProbeURL                 string   `json:"probeUrl"`
}

type gatewayBalancer struct {
	Tag         string   `json:"tag"`
	Selector    []string `json:"selector"`
	PrimaryTag  string   `json:"primaryTag"`
	FallbackTag string   `json:"fallbackTag"`
	Strategy    struct {
		Type     string          `json:"type"`
		Settings json.RawMessage `json:"settings"`
	} `json:"strategy"`
}

func GatewayConfigurationChanged(previous, proposed string) (bool, error) {
	gateways := func(template string) (map[string]map[string]any, error) {
		var data struct {
			Routing struct {
				Balancers []map[string]any
			}
		}
		if err := json.Unmarshal([]byte(UnwrapXrayTemplateConfig(template)), &data); err != nil {
			return nil, err
		}
		configured := map[string]map[string]any{}
		for _, balancer := range data.Routing.Balancers {
			strategy, _ := balancer["strategy"].(map[string]any)
			if strategyName, _ := strategy["type"].(string); strings.EqualFold(strategyName, gatewayStrategyName) {
				tag, _ := balancer["tag"].(string)
				if _, exists := configured[tag]; exists {
					return nil, fmt.Errorf("duplicate gateway tag %q", tag)
				}
				configured[tag] = balancer
			}
		}
		return configured, nil
	}
	current, err := gateways(previous)
	if err != nil {
		return false, err
	}
	next, err := gateways(proposed)
	return !reflect.DeepEqual(current, next), err
}

const defaultGatewayProbeURL = "https://www.google.com/generate_204"

func (b gatewayBalancer) settings() (gatewaySettings, error) {
	settings := gatewaySettings{ProbeURL: defaultGatewayProbeURL}
	err := json.Unmarshal(b.Strategy.Settings, &settings)
	return settings, err
}

func compileGatewayBalancers(cfg *xray.Config) error {
	if len(cfg.RouterConfig) == 0 {
		return nil
	}
	var routing map[string]json.RawMessage
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		return err
	}
	var balancers []json.RawMessage
	if raw := routing["balancers"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &balancers); err != nil {
			return err
		}
	}
	gateways := map[int]gatewayBalancer{}
	for index, raw := range balancers {
		var balancer gatewayBalancer
		if err := json.Unmarshal(raw, &balancer); err != nil {
			return err
		}
		if strings.EqualFold(balancer.Strategy.Type, gatewayStrategyName) {
			gateways[index] = balancer
		}
	}
	if len(gateways) == 0 {
		return nil
	}
	var outbounds []json.RawMessage
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		return err
	}
	tags := map[string]string{}
	for _, raw := range outbounds {
		var outbound struct{ Tag, Protocol string }
		if err := json.Unmarshal(raw, &outbound); err != nil {
			return err
		}
		tags[outbound.Tag] = outbound.Protocol
	}
	block := "vpnui-gateway-block"
	for {
		if _, exists := tags[block]; !exists {
			break
		}
		block += "-"
	}
	blackhole, _ := json.Marshal(map[string]string{"tag": block, "protocol": "blackhole"})
	outbounds = append(outbounds, blackhole)
	for index, b := range gateways {
		configuration, err := b.settings()
		if err != nil {
			return fmt.Errorf("gateway %q: %w", b.Tag, err)
		}
		normal := append([]string{b.PrimaryTag}, b.Selector...)
		if err := validateGateway(b.Tag, normal, configuration, tags); err != nil {
			return err
		}
		balancers[index], _ = json.Marshal(map[string]any{
			"tag": b.Tag, "selector": normal, "fallbackTag": block,
			"strategy": map[string]any{"type": gatewayStrategyName, "settings": configuration},
		})
	}
	var api map[string]json.RawMessage
	if err := json.Unmarshal(cfg.API, &api); err != nil {
		return fmt.Errorf("gateway requires the local Xray API: %w", err)
	}
	if api == nil {
		return fmt.Errorf("gateway requires the local Xray API")
	}
	var services []string
	if raw := api["services"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &services); err != nil {
			return err
		}
	}
	if !slices.Contains(services, "RoutingService") {
		api["services"], _ = json.Marshal(append(services, "RoutingService"))
	}
	cfg.API, _ = json.Marshal(api)
	routing["balancers"], _ = json.Marshal(balancers)
	cfg.RouterConfig, _ = json.Marshal(routing)
	cfg.OutboundConfigs, _ = json.Marshal(outbounds)
	return nil
}

func validateGateway(tag string, normal []string, settings gatewaySettings, outbounds map[string]string) error {
	if tag == "" || len(normal) < 2 || len(normal)+len(settings.EmergencyTags) > 32 {
		return fmt.Errorf("gateway %q requires a master and a backup, with at most 32 total outbounds", tag)
	}
	if settings.LatencyMS < 1 || settings.LatencyMS > 60000 ||
		settings.IntervalSeconds < 1 || settings.IntervalSeconds > 3600 ||
		settings.EmergencyIntervalSeconds < 1 || settings.EmergencyIntervalSeconds > 3600 ||
		settings.Attempts < 1 || settings.Attempts > 10 || settings.EmergencyAttempts < 1 || settings.EmergencyAttempts > 10 {
		return fmt.Errorf("gateway %q has invalid latency, interval or probe count", tag)
	}
	target, err := url.Parse(settings.ProbeURL)
	if err != nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return fmt.Errorf("gateway %q requires a valid HTTP or HTTPS probe URL", tag)
	}
	seen := map[string]bool{}
	for _, member := range append(slices.Clone(normal), settings.EmergencyTags...) {
		protocol, exists := outbounds[member]
		if member == "" || !exists || protocol == "blackhole" || protocol == "loopback" || seen[member] {
			return fmt.Errorf("gateway %q has an invalid or duplicate outbound %q", tag, member)
		}
		seen[member] = true
	}
	return nil
}

type GatewayStatus struct {
	Tag       string   `json:"tag"`
	Active    string   `json:"active"`
	Normal    []string `json:"normal"`
	Emergency []string `json:"emergency"`
	Available bool     `json:"available"`
}

type GatewayOverview struct {
	Outbounds []string        `json:"outbounds"`
	Balancers []GatewayStatus `json:"balancers"`
}

// GatewayStatus reports the running core's target, including a manual override.
func (s *XrayService) GatewayStatus(ctx context.Context) (GatewayOverview, error) {
	var out GatewayOverview
	lock.Lock()
	process := p
	var cfg *xray.Config
	var port int
	if process != nil && process.IsRunning() {
		cfg, port = process.GetConfig(), process.GetAPIPort()
	}
	lock.Unlock()
	compiled := cfg != nil
	if !compiled {
		template, err := s.settingService.GetXrayConfigTemplate()
		if err != nil {
			return out, err
		}
		cfg = &xray.Config{}
		if err := json.Unmarshal([]byte(template), cfg); err != nil {
			return out, err
		}
	}
	var outbounds []struct{ Tag string }
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		return out, err
	}
	var routing struct {
		Balancers []gatewayBalancer
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		return out, err
	}
	internal := map[string]bool{}
	if compiled {
		for _, b := range routing.Balancers {
			if b.Strategy.Type == gatewayStrategyName || b.Strategy.Type == "failover" {
				internal[b.FallbackTag] = true
			}
		}
	}
	for _, outbound := range outbounds {
		if outbound.Tag != "" && !internal[outbound.Tag] {
			out.Outbounds = append(out.Outbounds, outbound.Tag)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var api xray.XrayAPI
	connected := port > 0 && api.Init(port) == nil
	if connected {
		defer api.Close()
	}
	for _, b := range routing.Balancers {
		if !strings.EqualFold(b.Strategy.Type, gatewayStrategyName) {
			continue
		}
		settings, err := b.settings()
		if err != nil {
			return out, err
		}
		normal := b.Selector
		if !compiled {
			normal = append([]string{b.PrimaryTag}, normal...)
		}
		status := GatewayStatus{Tag: b.Tag, Normal: normal, Emergency: settings.EmergencyTags}
		if connected {
			status.Active, err = api.BalancerTarget(ctx, b.Tag)
			status.Available = err == nil
		}
		out.Balancers = append(out.Balancers, status)
	}
	return out, nil
}
