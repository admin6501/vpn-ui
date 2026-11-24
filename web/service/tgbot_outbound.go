package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

const telegramOutboundPrefix = "xray:"
const telegramInboundTag = "vpn-ui-telegram-proxy"

// Both the generated core config and the Telegram HTTP client use this
// process-local endpoint. Rebuilding the core config keeps the endpoint stable.
var telegramBridgeURL = sync.OnceValues(func() (string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "socks5", Host: address, User: url.UserPassword("telegram", uuid.NewString())}).String(), nil
})

func telegramOutboundTag(value string) (string, error) {
	tag, err := url.PathUnescape(strings.TrimPrefix(value, telegramOutboundPrefix))
	if err != nil || tag == "" {
		return "", errors.New("invalid Telegram Xray outbound tag")
	}
	return tag, nil
}

func telegramOutboundTags(config *xray.Config) ([]string, error) {
	var outbounds []struct {
		Tag      string
		Protocol string
	}
	if err := json.Unmarshal(config.OutboundConfigs, &outbounds); err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(outbounds))
	for _, outbound := range outbounds {
		if outbound.Tag != "" && outbound.Protocol != "blackhole" && outbound.Protocol != "loopback" {
			tags = append(tags, outbound.Tag)
		}
	}
	slices.Sort(tags)
	return slices.Compact(tags), nil
}

// TelegramOutboundTags returns only selectable tags, without outbound credentials.
func (s *XrayService) TelegramOutboundTags() ([]string, error) {
	template, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	var config xray.Config
	if err := json.Unmarshal([]byte(template), &config); err != nil {
		return nil, err
	}
	s.applySshOutbounds(&config)
	if err := applyVpnOutboundsWith(&config, (&VpnOutboundService{}).List()); err != nil {
		return nil, err
	}
	return telegramOutboundTags(&config)
}

func addTelegramOutboundBridge(config *xray.Config, tag string) error {
	tags, err := telegramOutboundTags(config)
	if err != nil {
		return err
	}
	if !slices.Contains(tags, tag) {
		return fmt.Errorf("Telegram Xray outbound %q is unavailable", tag)
	}
	for _, inbound := range config.InboundConfigs {
		if inbound.Tag == telegramInboundTag {
			return errors.New("Telegram proxy inbound tag is already in use")
		}
	}
	proxy, err := telegramBridgeURL()
	if err != nil {
		return err
	}
	endpoint, err := url.Parse(proxy)
	if err != nil {
		return err
	}
	port, err := net.LookupPort("tcp", endpoint.Port())
	if err != nil {
		return err
	}
	password, _ := endpoint.User.Password()
	settings, err := json.Marshal(map[string]any{
		"auth": "password", "accounts": []map[string]string{{"user": endpoint.User.Username(), "pass": password}},
	})
	if err != nil {
		return err
	}
	routing := make(map[string]json.RawMessage)
	if len(config.RouterConfig) != 0 {
		if err := json.Unmarshal(config.RouterConfig, &routing); err != nil {
			return err
		}
		if routing == nil {
			routing = make(map[string]json.RawMessage)
		}
	}
	var rules []json.RawMessage
	if raw := routing["rules"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &rules); err != nil {
			return err
		}
	}
	rule, err := json.Marshal(map[string]any{
		"type": "field", "inboundTag": []string{telegramInboundTag}, "outboundTag": tag,
	})
	if err != nil {
		return err
	}
	routing["rules"], err = json.Marshal(append([]json.RawMessage{rule}, rules...))
	if err != nil {
		return err
	}
	router, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	config.RouterConfig = router
	config.InboundConfigs = append(config.InboundConfigs, xray.InboundConfig{
		Listen: []byte(`"127.0.0.1"`), Port: port, Protocol: "socks", Tag: telegramInboundTag, Settings: settings,
	})
	return nil
}
