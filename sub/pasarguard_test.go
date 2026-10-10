package sub

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

// Exercises every migrated identity against the actual Gin subscription resolver.
// The supplied database is copied, so the user's staging artifact is untouched.
func TestPasarGuardImportedSampleTokens(t *testing.T) {
	source := os.Getenv("PG_MIGRATION_TEST_DB")
	if source == "" {
		t.Skip("set PG_MIGRATION_TEST_DB for sample-backup integration test")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(filename); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	var secret model.PasarGuardSecret
	if err := db.First(&secret, 1).Error; err != nil {
		t.Fatal(err)
	}
	var bindings []model.PasarGuardSubscription
	if err := db.Find(&bindings).Error; err != nil || len(bindings) == 0 {
		t.Fatal("missing bindings", err)
	}
	router := gin.New()
	generatedSubscriptions := 0
	router.GET("/sub/:subid", resolvePasarGuardSubscription, func(c *gin.Context) {
		var a model.Account
		if db.Where("sub_id = ?", c.Param("subid")).First(&a).Error != nil {
			c.AbortWithStatus(404)
			return
		}
		if a.Enable {
			links, _, _, err := NewSubService(false, "-i").GetSubs(a.SubID, "127.0.0.1")
			if err != nil || len(links) == 0 {
				c.AbortWithStatus(500)
				return
			}
			generatedSubscriptions++
		}
		c.Status(200)
	})
	for _, binding := range bindings {
		issued := binding.CreatedAt.Unix() + 1
		if binding.RevokedAt != nil && issued <= binding.RevokedAt.Unix() {
			issued = binding.RevokedAt.Unix() + 1
		}
		body := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v2,%d,%d", binding.SourceUserID, issued)))
		hash := sha256.Sum256([]byte(body + secret.SigningKey))
		token := body + hex.EncodeToString(hash[:])[:10]
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/"+token, nil))
		if rec.Code != 200 {
			t.Fatalf("imported identity %d returned status %d", binding.SourceUserID, rec.Code)
		}
		token = token[:len(token)-1] + "!"
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/"+token, nil))
		if rec.Code != 404 {
			t.Fatal("tampered token accepted")
		}
	}
	if generatedSubscriptions == 0 {
		t.Fatal("no active subscriptions generated")
	}
	// Rotation of the native subscription revokes all previously imported links.
	first := bindings[0]
	if err := db.Model(&model.Account{}).Where("id = ?", first.AccountID).Update("sub_id", "rotated-test-subscription").Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	body := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("v2,%d,%d", first.SourceUserID, now)))
	hash := sha256.Sum256([]byte(body + secret.SigningKey))
	token := body + hex.EncodeToString(hash[:])[:10]
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/"+token, nil))
	if rec.Code != 404 {
		t.Fatal("rotated subscription left legacy token active")
	}
}
