package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/xlzd/gotp"
)

func TestSecurityAdminUsernameCasePreserved(t *testing.T) {
	svc := newAdminDB(t)
	u, err := svc.AddAdmin(AdminSpec{Username: "  MiXeD.Operator  ", Password: "case-password", Enable: true})
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "MiXeD.Operator" {
		t.Fatalf("stored username = %q; want original case without surrounding spaces", u.Username)
	}
	password := u.Password
	if err := svc.UpdateAdmin(u.Id, AdminSpec{Username: u.Username, Nickname: "metadata edit", Enable: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateAdmin(u.Id, AdminSpec{Username: "MIXED.Operator", Enable: true}); err != nil {
		t.Fatalf("case-only rename of the same account: %v", err)
	}
	var stored model.User
	if err := database.GetDB().First(&stored, u.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != "MIXED.Operator" || stored.Password != password {
		t.Fatal("username edit did not preserve the requested case and existing password")
	}
	user, err := (&UserService{}).CheckUser(stored.Username, "case-password", "")
	if err != nil || user == nil || user.Id != u.Id {
		t.Fatalf("login with saved mixed-case username failed: %v", err)
	}
}

func TestSecurityRepresentativeUsernameCasePreserved(t *testing.T) {
	newAdminDB(t)
	owner, err := (&UserService{}).GetFirstUser()
	if err != nil {
		t.Fatal(err)
	}
	svc := &ResellerService{}
	u, err := svc.AddReseller(owner, ResellerSpec{Username: "  RePresentative.A  ", Password: "case-password", Enable: true})
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "RePresentative.A" {
		t.Fatalf("stored representative username = %q; want RePresentative.A", u.Username)
	}
	if err := svc.UpdateReseller(owner, u.Id, ResellerSpec{Username: u.Username, Nickname: "metadata edit", Enable: true}); err != nil {
		t.Fatal(err)
	}
	var stored model.User
	if err := database.GetDB().First(&stored, u.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != u.Username || stored.Password != u.Password {
		t.Fatal("representative metadata edit changed username case or password")
	}
	user, err := (&UserService{}).CheckUser(u.Username, "case-password", "")
	if err != nil || user == nil || user.Id != u.Id {
		t.Fatalf("representative cannot log in with the username entered at creation: %v", err)
	}
}

func TestSecurityRepresentativeEditPreservesImportedUsernameCase(t *testing.T) {
	newAdminDB(t)
	db := database.GetDB()
	owner, err := (&UserService{}).GetFirstUser()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := crypto.HashPasswordAsBcrypt("imported-password")
	if err != nil {
		t.Fatal(err)
	}
	const secret = "JBSWY3DPEHPK3PXP"
	u := model.User{Username: "Imported.Mixed", Password: hash, Enable: true, IsReseller: true, TwoFactorEnable: true, TwoFactorToken: secret}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerProfile{UserId: u.Id, CreatedBy: owner.Id, AllowanceBytes: 100 * gb}).Error; err != nil {
		t.Fatal(err)
	}
	login := func() {
		t.Helper()
		got, err := (&UserService{}).CheckUser(u.Username, "imported-password", gotp.NewDefaultTOTP(secret).Now())
		if err != nil || got == nil || got.Id != u.Id {
			t.Fatalf("original imported login failed: %v", err)
		}
	}
	login()
	if err := (&ResellerService{}).UpdateReseller(owner, u.Id, ResellerSpec{Username: u.Username, Nickname: "edited representative", Enable: true}); err != nil {
		t.Fatal(err)
	}
	var stored model.User
	if err := db.First(&stored, u.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Username != u.Username || stored.Password != hash || !stored.TwoFactorEnable || stored.TwoFactorToken != secret {
		t.Fatal("metadata edit changed imported login credentials or its authenticator")
	}
	login()
}

func TestSecurityUsernameCaseCollisionAcrossRoles(t *testing.T) {
	for _, name := range []string{"Imported.Mixed", "ÖPERATOR"} {
		t.Run(name, func(t *testing.T) {
			svc := newAdminDB(t)
			db := database.GetDB()
			owner, err := (&UserService{}).GetFirstUser()
			if err != nil {
				t.Fatal(err)
			}
			// Imported and CLI-created names can already contain uppercase letters.
			if err := db.Create(&model.User{Username: name, Password: "unused", Enable: true}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AddAdmin(AdminSpec{Username: strings.ToLower(name), Password: "case-password", Enable: true}); !errors.Is(err, ErrAdminUsernameTaken) {
				t.Fatalf("case-only administrator duplicate accepted: %v", err)
			}
			rs := &ResellerService{}
			if _, err := rs.AddReseller(owner, ResellerSpec{Username: strings.ToLower(name), Password: "case-password", Enable: true}); !errors.Is(err, ErrResellerNameTaken) {
				t.Fatalf("case-only representative duplicate accepted: %v", err)
			}
			admin, err := svc.AddAdmin(AdminSpec{Username: "Other.Admin", Password: "case-password", Enable: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: strings.ToLower(name), Enable: true}); !errors.Is(err, ErrAdminUsernameTaken) {
				t.Fatalf("case-only administrator rename collision accepted: %v", err)
			}
			rep, err := rs.AddReseller(owner, ResellerSpec{Username: "Other.Representative", Password: "case-password", Enable: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := rs.UpdateReseller(owner, rep.Id, ResellerSpec{Username: strings.ToLower(name), Enable: true}); !errors.Is(err, ErrResellerNameTaken) {
				t.Fatalf("case-only representative rename collision accepted: %v", err)
			}
		})
	}
}

func TestSecurityConcurrentUsernameCaseCreatesDoNotDuplicate(t *testing.T) {
	newAdminDB(t)
	owner, err := (&UserService{}).GetFirstUser()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 6; n++ {
		name := fmt.Sprintf("Concurrent.Case.%d", n)
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() {
			<-start
			_, err := (&AdminService{}).AddAdmin(AdminSpec{Username: name, Password: "case-password", Enable: true})
			done <- err
		}()
		go func() {
			<-start
			_, err := (&ResellerService{}).AddReseller(owner, ResellerSpec{Username: strings.ToLower(name), Password: "case-password", Enable: true})
			done <- err
		}()
		close(start)
		succeeded := 0
		for i := 0; i < 2; i++ {
			if <-done == nil {
				succeeded++
			}
		}
		var count int64
		if err := database.GetDB().Model(&model.User{}).Where("LOWER(username) = ?", strings.ToLower(name)).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if succeeded != 1 || count != 1 {
			t.Fatalf("concurrent case variants created %d rows (%d successful calls); want one", count, succeeded)
		}
	}
}
