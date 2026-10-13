package service

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

func validateNodeInboundIdentities(inbounds []model.Inbound) error {
	ids, tags := map[int]bool{}, map[string]bool{}
	for _, in := range inbounds {
		if in.Id <= 0 || ids[in.Id] || in.Tag == "" || tags[in.Tag] {
			return errors.New("Invalid or duplicate node inbound identity/tag")
		}
		ids[in.Id], tags[in.Tag] = true, true
	}
	return nil
}

// A removed inbound stays as an inactive meter tombstone. Free its old tag if
// a central inbound now owns that tag. The same reservation phase permits tag
// swaps between existing IDs without violating the unique index mid-update.
// IDs, counters, memberships and central routing tags are never rewritten.
func reserveNodeInboundTags(tx *gorm.DB, incoming []model.Inbound) error {
	owners := map[string]int{}
	used := map[string]bool{}
	for _, in := range incoming {
		owners[in.Tag] = in.Id
		used[in.Tag] = true
	}
	var rows []model.Inbound
	if err := tx.Select("id,tag").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		used[row.Tag] = true
	}
	for _, row := range rows {
		owner, claimed := owners[row.Tag]
		if !claimed || owner == row.Id {
			continue
		}
		var archived string
		for {
			var nonce [12]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				return err
			}
			archived = fmt.Sprintf("node-retired-inbound-%d-%x", row.Id, nonce)
			if !used[archived] {
				break
			}
		}
		used[archived] = true
		if err := tx.Model(&model.Inbound{}).Where("id = ?", row.Id).Update("tag", archived).Error; err != nil {
			return err
		}
	}
	return nil
}
