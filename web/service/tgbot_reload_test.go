package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

type telegramTestLanguage string

func (lang telegramTestLanguage) GetTgLang() (string, error) { return string(lang), nil }

func TestTelegramPersianCommandsRespondAfterLanguageReload(t *testing.T) {
	messages := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Error(err)
		}
		messages <- data.Text
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
	}))
	defer server.Close()
	client, err := telego.NewBot("123456789:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", telego.WithAPIServer(server.URL), telego.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	tgBotMutex.Lock()
	previous, wasRunning := bot, isRunning
	bot = client
	isRunning = true
	tgBotMutex.Unlock()
	defer func() { tgBotMutex.Lock(); bot = previous; isRunning = wasRunning; tgBotMutex.Unlock() }()
	var service Tgbot
	for _, lang := range []telegramTestLanguage{"en-US", "fa-IR", "en-US", "fa-IR"} {
		if err := locale.InitLocalizer(os.DirFS(".."), lang); err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"/start", "/status", "/id"} {
			service.answerCommand(&telego.Message{Text: command, From: &telego.User{ID: 1, FirstName: "Ali"}}, 1, false)
			select {
			case text := <-messages:
				if text == "" || !utf8.ValidString(text) || strings.Contains(text, "tgbot.commands.") {
					t.Fatalf("invalid bot response: %q", text)
				}
				if command == "/start" && lang == "fa-IR" && !strings.Contains(text, "سلام") {
					t.Fatalf("Persian greeting missing: %q", text)
				}
			case <-time.After(time.Second):
				t.Fatalf("no response in %s for %s", lang, command)
			}
		}
	}
}

func TestTelegramReloadCancelsReceiverBeforeWaitingForHandler(t *testing.T) {
	client, err := telego.NewBot("123456789:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan telego.Update, 1)
	handler, err := th.NewBotHandler(client, updates)
	if err != nil {
		t.Fatal(err)
	}
	receiverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	handler.HandleMessage(func(_ *th.Context, _ telego.Message) error { close(entered); <-receiverCtx.Done(); return nil }, th.AnyMessage())
	botWG.Add(1)
	go func() { defer botWG.Done(); _ = handler.Start() }()
	updates <- telego.Update{Message: &telego.Message{Text: "ping"}}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	tgBotMutex.Lock()
	botCancel = cancel
	botHandler = handler
	isRunning = true
	tgBotMutex.Unlock()
	done := make(chan struct{})
	go func() { StopBot(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("bot reload waited before cancelling receiver")
	}
	if (&Tgbot{}).IsRunning() {
		t.Fatal("receiver remains marked running after shutdown")
	}
}
