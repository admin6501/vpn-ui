package service

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// SetPeerIP changes only one router address. Keep the settings as raw JSON so
// account credentials, counters, pool slots and future protocol fields survive.
// expectedIP prevents a config window opened earlier from overwriting a newer edit.
func (s *GreService) SetPeerIP(inboundID int, email string, index int, peerIP, expectedIP string) error {
	if inboundID <= 0 || email == "" || index < 0 || index >= 64 {
		return fmt.Errorf("invalid GRE client or peer slot")
	}
	peerIP = strings.TrimSpace(peerIP)
	if peerIP != "" {
		ip := net.ParseIP(peerIP)
		if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() {
			return fmt.Errorf("enter a valid router IPv4 address, or leave it blank for dynamic addressing")
		}
		peerIP = ip.To4().String()
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var inbound model.Inbound
		if err := tx.First(&inbound, inboundID).Error; err != nil {
			return err
		}
		if inbound.Protocol != model.GRE {
			return fmt.Errorf("not a GRE inbound")
		}
		var settings map[string]json.RawMessage
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return err
		}
		var clients []map[string]json.RawMessage
		if err := json.Unmarshal(settings["clients"], &clients); err != nil {
			return err
		}
		found := false
		var updatedClient []byte
		for _, client := range clients {
			var storedEmail string
			if err := json.Unmarshal(client["email"], &storedEmail); err != nil {
				return err
			}
			if storedEmail != email {
				continue
			}
			var peers []map[string]json.RawMessage
			if err := json.Unmarshal(client["peers"], &peers); err != nil {
				return err
			}
			if index < 0 || index >= len(peers) {
				return fmt.Errorf("peer slot not found; reopen GRE setup")
			}
			var previous string
			if raw := peers[index]["peerIp"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &previous); err != nil {
					return err
				}
			}
			if strings.TrimSpace(previous) != strings.TrimSpace(expectedIP) {
				return fmt.Errorf("router IP changed since this window was opened; reopen GRE setup")
			}
			if peers[index] == nil {
				peers[index] = make(map[string]json.RawMessage)
			}
			peers[index]["peerIp"], _ = json.Marshal(peerIP)
			client["peers"], _ = json.Marshal(peers)
			updatedClient, _ = json.Marshal(client)
			found = true
			break
		}
		if !found {
			return fmt.Errorf("client not found on this GRE inbound")
		}
		settings["clients"], _ = json.Marshal(clients)
		updated, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		result := tx.Model(&model.Inbound{}).Where("id = ? AND settings = ?", inboundID, inbound.Settings).
			Update("settings", string(updated))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("inbound changed during save; reopen GRE setup")
		}
		// The membership's fallback copy must carry the same peers when a client
		// entry is later re-created. Do not sync account fields from this edit.
		accountIDs := tx.Model(&model.Account{}).Select("id").Where("LOWER(TRIM(email)) = ?", accountKey(email))
		return tx.Model(&model.AccountInbound{}).Where("inbound_id = ? AND account_id IN (?)", inboundID, accountIDs).
			Update("extra", string(updatedClient)).Error
	})
}
