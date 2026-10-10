// pasarguard-db initializes an isolated migration database without starting VPN services.
package main

import (
	"fmt"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pasarguard-db EMPTY_OUTPUT_DIRECTORY")
		os.Exit(2)
	}
	if err := os.MkdirAll(os.Args[1], 0700); err != nil {
		panic(err)
	}
	os.Setenv("VPNUI_DB_FOLDER", os.Args[1])
	if err := database.InitDB(filepath.Join(os.Args[1], "vpn-ui.db")); err != nil {
		panic(err)
	}
	template, err := (&service.SettingService{}).GetXrayConfigTemplate()
	if err != nil {
		panic(err)
	}
	var count int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "xrayTemplateConfig").Count(&count).Error; err != nil {
		panic(err)
	}
	if count == 0 {
		if err := database.GetDB().Create(&model.Setting{Key: "xrayTemplateConfig", Value: template}).Error; err != nil {
			panic(err)
		}
	}
	fmt.Println("Isolated database schema initialized; no panel or VPN services started.")
}
