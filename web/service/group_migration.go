package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const importedGroupsMigrationKey = "representativeGroupsMigrationV2"

// V2 is additive: restore original names/structure next to compatibility groups,
// then replace representatives' fixed inbound scope with an equal group union.
// Customer group links, credentials, traffic and quotas are never rewritten.
func (s *GroupService) migrateImportedGroupsAndGrants() error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var marker model.Setting
		if err := tx.Where("key = ?", importedGroupsMigrationKey).First(&marker).Error; err == nil && marker.Value == "1" {
			return nil
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var resources []model.PasarGuardResource
		if err := tx.Where("kind IN ?", []string{"groups", "inbounds", "inbounds_groups_association", "admin_roles"}).Order("kind, source_id").Find(&resources).Error; err != nil {
			return err
		}
		sourceTags := map[string]string{}
		sourceGroups := map[int]map[string]string{}
		sourceLinks := map[int][]string{}
		sourceRoles := map[int]map[string]string{}
		for _, r := range resources {
			var row map[string]string
			if err := json.Unmarshal([]byte(r.Data), &row); err != nil {
				return err
			}
			switch r.Kind {
			case "groups":
				sourceGroups[r.SourceID] = row
			case "inbounds":
				sourceTags[row["id"]] = row["tag"]
			case "inbounds_groups_association":
				id, _ := strconv.Atoi(row["group_id"])
				sourceLinks[id] = append(sourceLinks[id], row["inbound_id"])
			case "admin_roles":
				sourceRoles[r.SourceID] = row
			}
		}
		var inbounds []model.Inbound
		if err := tx.Order("id").Find(&inbounds).Error; err != nil {
			return err
		}
		byTag := map[string]int{}
		for _, ib := range inbounds {
			byTag[ib.Tag] = ib.Id
		}
		sourceIDs := []int{}
		for id := range sourceGroups {
			sourceIDs = append(sourceIDs, id)
		}
		sort.Ints(sourceIDs)
		restored := map[int]model.ClientGroup{}
		for _, sid := range sourceIDs {
			row := sourceGroups[sid]
			name := row["name"]
			if name == "" {
				name = fmt.Sprintf("PasarGuard #%d", sid)
			}
			id := sid
			g := model.ClientGroup{Name: name, PasarGuardID: &id, Disabled: row["is_disabled"] == "t" || row["is_disabled"] == "true" || row["is_disabled"] == "1"}
			if err := tx.Create(&g).Error; err != nil {
				return err
			}
			seen := map[int]bool{}
			for _, sourceInbound := range sourceLinks[sid] {
				iid := byTag[sourceTags[sourceInbound]]
				if iid == 0 || seen[iid] {
					continue
				}
				seen[iid] = true
				if err := tx.Create(&model.GroupInbound{GroupID: g.ID, InboundID: iid}).Error; err != nil {
					return err
				}
			}
			restored[sid] = g
		}
		// Preserve current role edits. Original groups can be added to a role only
		// when their complete inbound set already fits its current native policy.
		for rid, row := range sourceRoles {
			var original struct {
				Allowed *[]int `json:"allowed_group_ids"`
			}
			if err := json.Unmarshal([]byte(row["access"]), &original); err != nil {
				return err
			}
			if original.Allowed == nil {
				continue
			}
			var role model.RepresentativeRole
			if err := tx.First(&role, rid).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			} else if err != nil {
				return err
			}
			var access map[string]json.RawMessage
			if err := json.Unmarshal([]byte(role.Access), &access); err != nil {
				return err
			}
			var current *[]int
			if err := json.Unmarshal(access["allowed_group_ids"], &current); err != nil && len(access["allowed_group_ids"]) > 0 {
				return err
			}
			if current == nil {
				continue
			}
			allowedIB, err := groupInboundIDs(tx, *current)
			if err != nil {
				return err
			}
			scope := map[int]bool{}
			for _, id := range allowedIB {
				scope[id] = true
			}
			ids := append([]int{}, (*current)...)
			for _, sid := range *original.Allowed {
				g, ok := restored[sid]
				if !ok {
					continue
				}
				ibs, err := groupInboundIDs(tx, []int{g.ID})
				if err != nil {
					return err
				}
				fits := len(ibs) > 0
				for _, id := range ibs {
					fits = fits && scope[id]
				}
				if fits {
					ids = append(ids, g.ID)
				}
			}
			encoded, _ := json.Marshal(uniqueGroupIDs(ids))
			access["allowed_group_ids"] = encoded
			encoded, _ = json.Marshal(access)
			if err := tx.Model(&role).Update("access", string(encoded)).Error; err != nil {
				return err
			}
		}
		var users []model.User
		if err := tx.Order("id").Find(&users).Error; err != nil {
			return err
		}
		for _, u := range users {
			if !u.IsRepresentativeAccount() || u.GroupAccess {
				continue
			}
			old := []int{}
			if err := tx.Model(&model.InboundAccess{}).Where("user_id = ?", u.Id).Order("inbound_id").Pluck("inbound_id", &old).Error; err != nil {
				return err
			}
			scope := map[int]bool{}
			for _, id := range old {
				scope[id] = true
			}
			covered := map[int]bool{}
			grants := []int{}
			policy, err := roleGroupIDs(tx, &u)
			if err != nil {
				return err
			}
			roleAllowed := map[int]bool{}
			if policy != nil {
				for _, gid := range *policy {
					roleAllowed[gid] = true
				}
			}
			for _, sid := range sourceIDs {
				g := restored[sid]
				if g.Disabled || (policy != nil && !roleAllowed[g.ID]) {
					continue
				}
				ibs, err := groupInboundIDs(tx, []int{g.ID})
				if err != nil {
					return err
				}
				fits := len(ibs) > 0
				for _, id := range ibs {
					fits = fits && scope[id]
				}
				if fits {
					grants = append(grants, g.ID)
					for _, id := range ibs {
						covered[id] = true
					}
				}
			}
			for _, iid := range old {
				if covered[iid] {
					continue
				}
				var ib model.Inbound
				if err := tx.First(&ib, iid).Error; errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				} else if err != nil {
					return err
				}
				var g model.ClientGroup
				err := tx.Where("pasar_guard_id IS NULL AND disabled = 0 AND id IN (SELECT group_id FROM group_inbounds GROUP BY group_id HAVING COUNT(*)=1 AND MIN(inbound_id)=?)", iid).Order("id").First(&g).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					g = model.ClientGroup{Name: fmt.Sprintf("%s (#%d)", ib.Tag, iid)}
					if err := tx.Create(&g).Error; err != nil {
						return err
					}
					if err := tx.Create(&model.GroupInbound{GroupID: g.ID, InboundID: iid}).Error; err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
				grants = append(grants, g.ID)
			}
			if err := setRepresentativeGroups(tx, u.Id, grants); err != nil {
				return err
			}
		}
		return tx.Create(&model.Setting{Key: importedGroupsMigrationKey, Value: "1"}).Error
	})
}
