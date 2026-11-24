package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type stockClientMembership struct {
	Email        string
	SubID        string `gorm:"column:sub_id"`
	Password     string
	Auth         string
	Flow         string
	Security     string
	LimitIP      int   `gorm:"column:limit_ip"`
	TotalGB      int64 `gorm:"column:total_gb"`
	ExpiryTime   int64
	Enable       bool
	TgID         int64 `gorm:"column:tg_id"`
	Comment      string
	Reset        int
	CreatedAt    int64
	UpdatedAt    int64
	UUID         string
	InboundID    int
	FlowOverride string
	PrivateKey   string `gorm:"column:wg_private_key"`
	PublicKey    string `gorm:"column:wg_public_key"`
	PreSharedKey string `gorm:"column:wg_pre_shared_key"`
	AllowedIPs   string `gorm:"column:wg_allowed_ips"`
}

// Recent 3x-ui backups store clients and their inbound membership in separate
// tables. VPN-UI's account backfill reads settings.clients, so reconstruct that
// projection on the private staging copy before opening it as the panel database.
func prepareStock3xUIDatabase(path string) error {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	for _, table := range []string{"users", "settings", "inbounds"} {
		if !db.Migrator().HasTable(table) {
			return fmt.Errorf("backup is missing the %s table", table)
		}
	}
	hasClients := db.Migrator().HasTable("clients")
	hasMemberships := db.Migrator().HasTable("client_inbounds")
	if hasClients != hasMemberships {
		return fmt.Errorf("backup is missing a 3x-ui client or membership table")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("accounts") && tx.Migrator().HasTable("client_traffics") {
			if err := normalizeStockTrafficTable(tx); err != nil {
				return fmt.Errorf("preparing 3x-ui client traffic: %w", err)
			}
		}
		if !hasClients {
			return nil
		}
		var inbounds []struct {
			ID       int
			Protocol model.Protocol
			Settings string
		}
		if err := tx.Table("inbounds").Select("id, protocol, settings").Find(&inbounds).Error; err != nil {
			return err
		}
		var links []stockClientMembership
		override := "''"
		if tx.Migrator().HasColumn("client_inbounds", "flow_override") {
			override = "links.flow_override"
		}
		if err := tx.Table("client_inbounds AS links").
			Select("clients.*, links.inbound_id, " + override + " AS flow_override").
			Joins("JOIN clients ON clients.id = links.client_id").
			Order("links.inbound_id, links.client_id").Scan(&links).Error; err != nil {
			return err
		}
		var linkCount int64
		if err := tx.Table("client_inbounds").Count(&linkCount).Error; err != nil {
			return err
		}
		if int64(len(links)) != linkCount {
			return fmt.Errorf("3x-ui backup has a client membership without a client")
		}
		byInbound := make(map[int][]stockClientMembership)
		for _, link := range links {
			byInbound[link.InboundID] = append(byInbound[link.InboundID], link)
		}
		for _, inbound := range inbounds {
			members := byInbound[inbound.ID]
			if len(members) == 0 {
				continue
			}
			var settings map[string]json.RawMessage
			if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || settings == nil {
				return fmt.Errorf("invalid settings for 3x-ui inbound %d", inbound.ID)
			}
			var existing []map[string]json.RawMessage
			if raw := settings["clients"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &existing); err != nil {
					return fmt.Errorf("reading clients of 3x-ui inbound %d: %w", inbound.ID, err)
				}
			}
			index := make(map[string]int)
			for i, client := range existing {
				var email string
				if json.Unmarshal(client["email"], &email) == nil {
					index[strings.ToLower(strings.TrimSpace(email))] = i
				}
			}
			for _, member := range members {
				client := model.Client{
					ID: member.UUID, Email: member.Email, SubID: member.SubID,
					Password: member.Password, Auth: member.Auth, Security: member.Security, Flow: member.Flow,
					LimitIP: member.LimitIP, TotalGB: member.TotalGB, ExpiryTime: member.ExpiryTime,
					Enable: member.Enable, TgID: member.TgID, Comment: member.Comment, Reset: member.Reset,
					CreatedAt: member.CreatedAt, UpdatedAt: member.UpdatedAt,
				}
				if member.FlowOverride != "" {
					client.Flow = member.FlowOverride
				}
				switch inbound.Protocol {
				case model.VMESS, model.VLESS, model.Trojan, model.Shadowsocks, model.Hysteria, model.Hysteria2, model.TUIC:
				case model.WireGuard:
					client.PrivKey, client.PubKey, client.Psk = member.PrivateKey, member.PublicKey, member.PreSharedKey
					client.Addresses = strings.FieldsFunc(member.AllowedIPs, func(r rune) bool { return r == ',' || r == ' ' })
				default:
					return fmt.Errorf("3x-ui inbound %d uses unsupported client migration protocol %q", inbound.ID, inbound.Protocol)
				}
				key := strings.ToLower(strings.TrimSpace(client.Email))
				if key == "" {
					return fmt.Errorf("3x-ui inbound %d has a client without an email", inbound.ID)
				}
				data, err := json.Marshal(client)
				if err != nil {
					return err
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(data, &fields); err != nil {
					return err
				}
				if i, ok := index[key]; ok {
					for field, value := range fields {
						existing[i][field] = value
					}
				} else {
					index[key] = len(existing)
					existing = append(existing, fields)
				}
			}
			raw, err := json.Marshal(existing)
			if err != nil {
				return err
			}
			settings["clients"] = raw
			data, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Table("inbounds").Where("id = ?", inbound.ID).Update("settings", string(data)).Error; err != nil {
				return err
			}
			delete(byInbound, inbound.ID)
		}
		if len(byInbound) > 0 {
			return fmt.Errorf("3x-ui backup has a client membership without an inbound")
		}
		return nil
	})
}

// SQLite's GORM table rebuild can omit columns in a foreign CREATE TABLE
// statement with formatting it doesn't recognize. Copy the columns by their
// SQLite names into the native schema before AutoMigrate runs.
func normalizeStockTrafficTable(tx *gorm.DB) error {
	columns, err := tx.Migrator().ColumnTypes("client_traffics")
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(columns))
	for _, column := range columns {
		present[column.Name()] = true
	}
	for _, required := range []string{"id", "inbound_id", "email", "up", "down"} {
		if !present[required] {
			return fmt.Errorf("client traffic table is missing %s", required)
		}
	}
	supported := []string{"id", "inbound_id", "email", "up", "down", "enable", "total", "all_time", "expiry_time", "reset", "last_online"}
	var copied []string
	for _, name := range supported {
		if present[name] {
			copied = append(copied, name)
		}
	}
	if err := tx.Exec("CREATE TEMP TABLE vpnui_import_traffic AS SELECT * FROM client_traffics").Error; err != nil {
		return err
	}
	if err := tx.Migrator().DropTable(&xray.ClientTraffic{}); err != nil {
		return err
	}
	if err := tx.Migrator().CreateTable(&xray.ClientTraffic{}); err != nil {
		return err
	}
	fields := strings.Join(copied, ", ")
	if err := tx.Exec("INSERT INTO client_traffics (" + fields + ") SELECT " + fields + " FROM vpnui_import_traffic").Error; err != nil {
		return err
	}
	return tx.Exec("DROP TABLE vpnui_import_traffic").Error
}
