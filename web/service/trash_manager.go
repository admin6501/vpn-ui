package service

import (
	"errors"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// DeleteManager archives the account and its clients in one transaction. Owned
// inbounds stay in service under the panel owner while the administrator is gone.
func (TrashService) DeleteManager(actor *model.User, id int, representative bool, mode string) ([]int, int, error) {
	if actor == nil || actor.Id == id || (!representative && !actor.IsPanelOwner) {
		return nil, 0, ErrProtectedAdmin
	}
	var touched []int
	deleted := 0
	keepClients := mode == DeleteModeKeep
	err := database.SnapshotTransaction(database.GetDB(), func(tx *gorm.DB) error {
		touched = nil
		deleted = 0
		var target model.User
		if err := tx.First(&target, id).Error; err != nil {
			return err
		}
		if target.IsPanelOwner || target.IsReseller != representative {
			return ErrProtectedAdmin
		}
		if target.IsSuperAdmin {
			last, err := (&AdminService{}).isLastSuperAdmin(tx, id)
			if err != nil {
				return err
			}
			if last {
				return ErrLastSuperAdmin
			}
		}
		if representative && !actor.IsPanelOwner && !actor.IsSuperAdmin {
			if err := trashRequireRows(tx, "reseller_profiles", "user_id = ? AND created_by = ?", id, actor.Id); err != nil {
				return err
			}
		}
		if representative && mode != DeleteModeKeep && mode != DeleteModeCascade {
			var clients int64
			if err := tx.Model(&model.ResellerClient{}).Where("user_id = ?", id).Count(&clients).Error; err != nil {
				return err
			}
			if clients > 0 {
				return ErrResellerHasClients
			}
		}
		if representative && !keepClients {
			var emails []string
			if err := tx.Model(&model.ResellerClient{}).Where("user_id = ?", id).
				Where("NOT EXISTS (SELECT 1 FROM accounts WHERE LOWER(TRIM(accounts.email)) = LOWER(TRIM(reseller_clients.email)))").
				Pluck("email", &emails).Error; err != nil {
				return err
			}
			ids, err := servingInboundIds(tx, emails...)
			if err != nil {
				return err
			}
			for _, inboundID := range ids {
				if err := (&AccountService{}).SyncInboundAccounts(tx, inboundID); err != nil {
					return err
				}
			}
		}
		var err error
		touched, deleted, err = archiveAndRemoveManager(tx, target, keepClients)
		return err
	})
	return touched, deleted, err
}

func archiveAndRemoveManager(tx *gorm.DB, target model.User, keepClients bool) ([]int, int, error) {
	id := target.Id
	var ownerID int
	if err := tx.Model(&model.User{}).Where("is_panel_owner = ?", true).Select("id").Scan(&ownerID).Error; err != nil {
		return nil, 0, err
	}
	users, err := trashRows(tx, "users", "id = ?", id)
	if err != nil {
		return nil, 0, err
	}
	if len(users) != 1 {
		return nil, 0, gorm.ErrRecordNotFound
	}
	snapshot := trashSnapshot{User: users[0]}
	parts := []struct {
		table string
		rows  *[]trashRow
	}{
		{"inbound_accesses", &snapshot.InboundGrants}, {"admin_group_accesses", &snapshot.AdminGroups},
		{"representative_group_accesses", &snapshot.RepresentativeGroups}, {"reseller_profiles", &snapshot.Profile},
		{"panel_activity_reads", &snapshot.ReadPositions},
	}
	for _, part := range parts {
		*part.rows, err = trashRows(tx, part.table, "user_id = ?", id)
		if err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Table("inbounds").Where("user_id = ?", id).Select("id, protocol").Scan(&snapshot.OwnedInbounds).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Model(&model.ResellerProfile{}).Where("created_by = ? AND user_id <> ?", id, id).
		Pluck("user_id", &snapshot.ManagedRepresentatives).Error; err != nil {
		return nil, 0, err
	}
	var accounts []model.Account
	if err := tx.Table("accounts a").Where("("+database.ActivityOwnerSQL+") = ?", id).Find(&accounts).Error; err != nil {
		return nil, 0, err
	}
	if keepClients {
		// Preserve only the assignments in keep mode. The live accounts can continue
		// changing while the reseller is gone and their service data stays live.
		snapshot.HandedOverClients, err = trashRows(tx, "reseller_clients", "user_id = ?", id)
		if err != nil {
			return nil, 0, err
		}
		for _, row := range snapshot.HandedOverClients {
			row["trash_account_id"] = int64(0)
			for _, account := range accounts {
				if accountKey(row.string("email")) == accountKey(account.Email) {
					row["trash_account_id"] = int64(account.Id)
					break
				}
			}
		}
	} else {
		for _, account := range accounts {
			client, err := snapshotTrashClient(tx, account.Id)
			if err != nil {
				return nil, 0, err
			}
			snapshot.Clients = append(snapshot.Clients, client)
		}
	}
	if err := saveTrash(tx, ownerID, TrashManager, target.Username, snapshot); err != nil {
		return nil, 0, err
	}
	var touched []int
	if !keepClients {
		for _, account := range accounts {
			ids, err := removeTrashClient(tx, account)
			if err != nil {
				return nil, 0, err
			}
			touched = append(touched, ids...)
		}
	}
	if err := tx.Model(&model.Account{}).Where("owner_admin_id = ?", id).UpdateColumn("owner_admin_id", ownerID).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Model(&model.Inbound{}).Where("user_id = ?", id).UpdateColumn("user_id", ownerID).Error; err != nil {
		return nil, 0, err
	}
	if err := tx.Model(&model.ResellerProfile{}).Where("created_by = ?", id).UpdateColumn("created_by", ownerID).Error; err != nil {
		return nil, 0, err
	}
	for _, table := range []string{"reseller_clients", "inbound_accesses", "admin_group_accesses",
		"representative_group_accesses", "reseller_profiles", "panel_activity_reads"} {
		if err := tx.Table(table).Where("user_id = ?", id).Delete(map[string]any{}).Error; err != nil {
			return nil, 0, err
		}
	}
	return touched, len(snapshot.Clients), tx.Delete(&target).Error
}

func restoreTrashManager(tx *gorm.DB, actor *model.User, s *trashSnapshot) error {
	id, name := s.User.integer("id"), s.User.string("username")
	if id <= 0 || name == "" || !actor.IsPanelOwner || s.User["is_panel_owner"] == true {
		return ErrTrashConflict
	}
	var count int64
	if err := tx.Model(&model.User{}).Where("id = ? OR LOWER(TRIM(username)) = ?", id, accountKey(name)).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrTrashConflict
	}
	for _, role := range []struct{ key, table string }{
		{"admin_role_id", "admin_roles"}, {"representative_role_id", "representative_roles"},
	} {
		if roleID := s.User.integer(role.key); roleID > 0 {
			if err := trashRequireRows(tx, role.table, "id = ?", roleID); err != nil {
				return err
			}
		}
	}
	for _, grant := range s.InboundGrants {
		if err := trashRequireRows(tx, "inbounds", "id = ?", grant.integer("inbound_id")); err != nil {
			return err
		}
	}
	for _, groups := range [][]trashRow{s.AdminGroups, s.RepresentativeGroups} {
		for _, group := range groups {
			if err := trashRequireRows(tx, "client_groups", "id = ?", group.integer("group_id")); err != nil {
				return err
			}
		}
	}
	for _, profile := range s.Profile {
		if manager := profile.integer("created_by"); manager != 0 {
			if err := trashRequireRows(tx, "users", "id = ? AND NOT is_reseller", manager); err != nil {
				return err
			}
		}
	}
	// Restoring the password and the user ID must not reactivate old cookies.
	s.User["session_version"] = s.User.integer("session_version") + 1
	s.User["is_panel_owner"] = false
	if err := insertTrashRows(tx, "users", s.User); err != nil {
		return err
	}
	for table, rows := range map[string][]trashRow{
		"inbound_accesses": s.InboundGrants, "admin_group_accesses": s.AdminGroups,
		"representative_group_accesses": s.RepresentativeGroups, "reseller_profiles": s.Profile,
		"panel_activity_reads": s.ReadPositions,
	} {
		if err := insertTrashRows(tx, table, rows...); err != nil {
			return err
		}
	}
	for _, ref := range s.OwnedInbounds {
		result := tx.Model(&model.Inbound{}).Where("id = ? AND protocol = ? AND user_id = ?", ref.ID, ref.Protocol, actor.Id).
			UpdateColumn("user_id", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTrashConflict
		}
	}
	for _, representative := range s.ManagedRepresentatives {
		result := tx.Model(&model.ResellerProfile{}).Where("user_id = ? AND created_by = ?", representative, actor.Id).
			UpdateColumn("created_by", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTrashConflict
		}
	}
	for _, row := range s.HandedOverClients {
		accountID := row.integer("trash_account_id")
		if accountID == 0 {
			continue // stale ownership record had no live account at deletion
		}
		email := accountKey(row.string("email"))
		var account model.Account
		err := tx.Where("id = ? AND LOWER(TRIM(email)) = ? AND owner_admin_id IN ?", accountID, email, []int{0, actor.Id}).First(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTrashConflict
		}
		if err != nil {
			return err
		}
		delete(row, "trash_account_id")
		if err := insertTrashRows(tx, "reseller_clients", row); err != nil {
			return err
		}
	}
	return nil
}
