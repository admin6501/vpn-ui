package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	tu "github.com/mymmrac/telego/telegoutil"
)

func TestTelegramOutboundBridgeRoutesThroughSelectedTag(t *testing.T) {
	cfg := &xray.Config{
		OutboundConfigs: []byte(`[{"tag":"direct","protocol":"freedom"},{"tag":"edge \u0026 one","protocol":"vless"},{"tag":"deny","protocol":"blackhole"}]`),
		RouterConfig:    []byte(`{"domainStrategy":"AsIs","rules":[{"type":"field","network":"tcp","outboundTag":"direct"}]}`),
	}
	tag, err := telegramOutboundTag("xray:edge%20%26%20one")
	if err != nil {
		t.Fatal(err)
	}
	if err := addTelegramOutboundBridge(cfg, "deny"); err == nil || len(cfg.InboundConfigs) != 0 {
		t.Fatal("blackhole outbound must be unavailable")
	}
	if err := addTelegramOutboundBridge(cfg, tag); err != nil {
		t.Fatal(err)
	}
	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("unexpected inbound count: %d", len(cfg.InboundConfigs))
	}
	inbound := cfg.InboundConfigs[0]
	if string(inbound.Listen) != `"127.0.0.1"` || inbound.Protocol != "socks" ||
		inbound.Port == 0 || !strings.Contains(string(inbound.Settings), `"auth":"password"`) {
		t.Fatalf("unexpected bridge fields: listen=%s protocol=%s port=%d", inbound.Listen, inbound.Protocol, inbound.Port)
	}
	var routing struct {
		DomainStrategy string
		Rules          []struct {
			InboundTag  []string
			OutboundTag string
		}
	}
	if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
		t.Fatal(err)
	}
	if len(routing.Rules) != 2 || len(routing.Rules[0].InboundTag) != 1 ||
		routing.Rules[0].InboundTag[0] != telegramInboundTag || routing.Rules[0].OutboundTag != tag ||
		routing.Rules[1].OutboundTag != "direct" || routing.DomainStrategy != "AsIs" {
		t.Fatalf("unexpected routing: %+v", routing)
	}
}

func TestTelegramClientDraftSerializesPanelClient(t *testing.T) {
	svc := &Tgbot{draft: &telegramClientDraft{
		Client:         model.Client{Email: "client", TgID: 123, Comment: "quoted \"note\"\nnext line", Enable: true},
		trojanPassword: "password-with-\"quotes\"",
	}}
	data, err := svc.BuildJSONForProtocol(model.Trojan)
	if err != nil {
		t.Fatal(err)
	}
	clients, err := (&InboundService{}).GetClients(&model.Inbound{Settings: data})
	if err != nil || len(clients) != 1 || clients[0].Comment != svc.draft.Comment ||
		clients[0].Password != svc.draft.trojanPassword || clients[0].TgID != 123 {
		t.Fatalf("client did not round-trip: %+v, %v", clients, err)
	}
}

func TestTelegramMenuAddsMainNavigation(t *testing.T) {
	bot := &Tgbot{}
	original := tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton("Refresh").WithCallbackData("refresh"),
	))
	menu := bot.withMainMenu(original)
	if len(original.InlineKeyboard) != 1 || len(menu.InlineKeyboard) != 2 ||
		menu.InlineKeyboard[1][0].CallbackData != telegramMainMenu {
		t.Fatal("main navigation must be added without mutating the supplied keyboard")
	}
}

// Set VPNUI_TEST_XRAY to an Xray executable to verify the real SOCKS listener
// and route with a local Bot API fixture.
func TestTelegramOutboundThroughXray(t *testing.T) {
	binary := os.Getenv("VPNUI_TEST_XRAY")
	if binary == "" {
		t.Skip("VPNUI_TEST_XRAY is not set")
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			t.Errorf("unexpected Bot API path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{"id":123456789,"is_bot":true,"first_name":"Fixture"}}`)
	}))
	defer api.Close()

	cfg := &xray.Config{
		OutboundConfigs: []byte(fmt.Sprintf(`[
			{"tag":"default","protocol":"blackhole"},
			{"tag":"selected","protocol":"freedom","settings":{"redirect":%q}}]`, api.Listener.Addr().String())),
	}
	if err := addTelegramOutboundBridge(cfg, "selected"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "xray.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	process := exec.Command(binary, "-c", path)
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	address := fmt.Sprintf("127.0.0.1:%d", cfg.InboundConfigs[0].Port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Xray did not start the SOCKS listener: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	// The target is intentionally unreachable directly. Only the selected
	// outbound redirects it to the local API fixture.
	bot, err := (&Tgbot{}).NewBot("123456789:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "xray:selected", "http://telegram-fixture.invalid")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	me, err := bot.GetMe(ctx)
	if err != nil || me.FirstName != "Fixture" {
		t.Fatalf("Telegram request did not reach the configured outbound: %v", err)
	}
}

func TestTelegramClientChangesReachPanelAccounts(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	inbound := model.Inbound{
		Tag: "test-telegram", Port: 34443, Protocol: model.VLESS, Settings: `{"clients":[]}`,
	}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	bot := &Tgbot{draft: &telegramClientDraft{
		inboundID: inbound.Id,
		Client: model.Client{
			ID: "12345678-1234-4234-9234-123456789abc", Email: "bot-account", SubID: "bot-sub", Enable: true,
		},
	}}
	if _, err := bot.SubmitAddClient(); err != nil {
		t.Fatal(err)
	}
	accounts := &AccountService{}
	account, err := accounts.GetAccountByEmail("bot-account")
	if err != nil || account == nil {
		t.Fatalf("account missing after Telegram creation: %+v, %v", account, err)
	}
	const telegramID = 102030405
	usage, err := bot.inboundService.GetClientTrafficByEmail("bot-account")
	if err != nil || usage == nil {
		t.Fatalf("traffic record missing: %+v, %v", usage, err)
	}
	if _, err := bot.inboundService.SetClientTelegramUserID(usage.Id, telegramID); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.inboundService.ResetClientTrafficLimitByEmail("bot-account", 3); err != nil {
		t.Fatal(err)
	}
	account, err = accounts.GetAccountByEmail("bot-account")
	if err != nil || account == nil || account.TgID != telegramID || account.TotalGB != 3*1024*1024*1024 {
		t.Fatalf("canonical account did not reflect Telegram edits: %+v, %v", account, err)
	}
	if !bot.canAccessTelegramClient(telegramID, "bot-account") || bot.canAccessTelegramClient(telegramID+1, "bot-account") {
		t.Fatal("subscription access does not follow the assigned Telegram identity")
	}
	traffics, err := bot.inboundService.GetClientTrafficTgBot(telegramID)
	if err != nil || len(traffics) != 1 || traffics[0].Email != "bot-account" {
		t.Fatalf("Telegram identity could not find its account: %+v, %v", traffics, err)
	}
}
