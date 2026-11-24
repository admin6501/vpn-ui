package service

import (
	"fmt"
	"io"
	"os"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/gorm"
)

// Importing a stock 3x-ui database is a wholesale swap, not a row-by-row merge:
// VPN-UI stages compatibility changes before opening the backup. InitDB carries
// the adopt migrations (promote the lone admin to super admin, adopt ownerless
// inbounds, move global 2FA onto the user) that turn a single-admin stock DB into
// this fork's model. So the whole file replaces the current one and InitDB does
// the translation, exactly the way the backup/restore path (ImportDB) already
// works. See database/db.go InitDB.
//
// The one thing a stock backup must NOT bring with it is how THIS panel is reached
// and how it authenticates: its port, path, listen address, TLS certs, session
// secret, systemd unit name, RADIUS secret and provisioning state describe the
// running install, not the operator's data. Importing a backup's copies of those
// would move the panel out from under the admin doing the import (new port, dead
// TLS, logged-out sessions, broken VPN auth). So they are snapshotted before the
// swap and written back after it; everything else (inbounds, clients, traffic,
// admins, subscription content, Telegram/LDAP config, the xray template with the
// operator's own outbounds and routing) comes across from the backup.
var preservedSettingKeys = []string{
	// How the panel is reached.
	"webListen", "webDomain", "webPort", "webCertFile", "webKeyFile", "webBasePath",
	"subListen", "subDomain", "subPort", "subCertFile", "subKeyFile",
	"subPath", "subJsonPath", "subClashPath",
	// How it authenticates / identifies itself.
	"secret", "sessionMaxAge", "systemdServiceName", "radiusSecret",
	// What has been provisioned on THIS host.
	"vpnProvisioned", "provisionedProtocols",
}

// vpnRangeProtocols are the tunnelled protocols that own per-inbound /24 ranges.
// After importing a vpn-ui backup that carries them, the ranges are re-normalized
// so ownership is non-overlapping on this host. A no-op for a stock 3x-ui backup,
// which has none of these inbounds.
var vpnRangeProtocols = []string{"l2tp", "pptp", "openvpn", "openconnect", "sstp", "ikev2", "wg-c", "awg", "gre"}

// dbFile is what both callers of ImportForeignDB provide: an uploaded multipart
// file (panel) or an *os.File (CLI). Both can be read, seeked, and read-at, which
// is all the signature check and the copy-to-temp need.
type dbFile interface {
	io.Reader
	io.ReaderAt
	io.Seeker
}

// ImportReport says what an import landed, for the operator and the dry-run preview.
type ImportReport struct {
	Inbounds          int      `json:"inbounds"`
	Clients           int      `json:"clients"`
	Admins            int      `json:"admins"`
	PreservedSettings []string `json:"preservedSettings"`
}

// ImportForeignDB replaces the current database with an uploaded 3x-ui (or vpn-ui)
// backup, preserving this panel's reachability/identity settings (see
// preservedSettingKeys). activate regenerates daemon configs and restarts Xray; a
// fresh CLI install passes false because the panel is not running yet and its own
// startup will bring services up against the imported data.
//
// The swap is crash-safe: the current DB is moved aside to a .backup and restored
// if any step through InitDB fails, so a failed import never leaves the panel with
// no database. Mirrors ImportDB, which this shares its swap shape with.
func (s *ServerService) ImportForeignDB(src dbFile, activate bool) (*ImportReport, error) {
	databaseImportMu.Lock()
	defer databaseImportMu.Unlock()
	// Validate the upload is actually a SQLite file before touching anything.
	isValidDb, err := database.IsSQLiteDB(src)
	if err != nil {
		return nil, common.NewErrorf("Error checking db file format: %v", err)
	}
	if !isValidDb {
		return nil, common.NewError("Invalid db file format")
	}
	if _, err = src.Seek(0, io.SeekStart); err != nil {
		return nil, common.NewErrorf("Error resetting file reader: %v", err)
	}

	// Snapshot the settings that must survive the swap, from the live DB, while it
	// is still the current one. Effective values (a DB row, else the built-in
	// default) so a key that was never written still overwrites the backup's copy.
	preserved, err := s.snapshotPreservedSettings()
	if err != nil {
		return nil, fmt.Errorf("reading current panel settings: %w", err)
	}

	dbPath := config.GetDBPath()
	tempPath := fmt.Sprintf("%s.temp", dbPath)

	if _, err := os.Stat(tempPath); err == nil {
		if errRemove := os.Remove(tempPath); errRemove != nil {
			return nil, common.NewErrorf("Error removing existing temporary db file: %v", errRemove)
		}
	}
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, common.NewErrorf("Error creating temporary db file: %v", err)
	}
	defer func() {
		if tempFile != nil {
			if cerr := tempFile.Close(); cerr != nil {
				logger.Warningf("import: failed to close temp file: %v", cerr)
			}
		}
		if _, err := os.Stat(tempPath); err == nil {
			if rerr := os.Remove(tempPath); rerr != nil {
				logger.Warningf("import: failed to remove temp file: %v", rerr)
			}
		}
	}()

	if _, err = io.Copy(tempFile, src); err != nil {
		return nil, common.NewErrorf("Error saving db: %v", err)
	}
	if err = tempFile.Close(); err != nil {
		return nil, common.NewErrorf("Error closing temporary db file: %v", err)
	}
	tempFile = nil

	// Structural integrity of the upload, before it can replace a working DB.
	if err = database.ValidateSQLiteDB(tempPath); err != nil {
		return nil, common.NewErrorf("Invalid or corrupt db file: %v", err)
	}

	if err := prepareStock3xUIDatabase(tempPath); err != nil {
		return nil, fmt.Errorf("preparing 3x-ui backup: %w", err)
	}

	fallbackPath := fmt.Sprintf("%s.backup", dbPath)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("locating current database: %w", err)
	}
	if err = os.Remove(fallbackPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing previous fallback: %w", err)
	}
	if activate && s.xrayService.IsXrayRunning() {
		if err := s.StopXrayService(); err != nil {
			return nil, fmt.Errorf("stopping Xray before import: %w", err)
		}
	}
	if err := database.CloseDB(); err != nil {
		if activate {
			_ = s.RestartXrayService()
		}
		return nil, fmt.Errorf("closing database before import: %w", err)
	}
	if err := os.Rename(dbPath, fallbackPath); err != nil {
		if reopenErr := database.InitDB(dbPath); reopenErr != nil {
			return nil, fmt.Errorf("backing up database: %v; reopening current database: %w", err, reopenErr)
		}
		if activate {
			_ = s.RestartXrayService()
		}
		return nil, fmt.Errorf("backing up database: %w", err)
	}
	database.RemoveSidecars(dbPath)
	rollback := func(cause error) (*ImportReport, error) {
		return nil, s.rollbackDatabaseImport(fallbackPath, cause, activate)
	}
	if err := os.Rename(tempPath, dbPath); err != nil {
		return rollback(fmt.Errorf("moving uploaded database: %w", err))
	}
	if err := database.InitDB(dbPath); err != nil {
		return rollback(fmt.Errorf("migrating uploaded database: %w", err))
	}

	// Put this panel's reachability/identity settings back over the backup's.
	if err = s.restorePreservedSettings(preserved); err != nil {
		return rollback(fmt.Errorf("preserving panel settings: %w", err))
	}

	s.inboundService.MigrateDB()

	report := s.buildImportReport(preserved)

	if activate {
		// Bring the ACCOUNTS layer up for the data that just arrived.
		//
		// MigrateDB above does not do this, and neither did anything else on this
		// path: MigrationAccounts and the wireguard reconcile are only called from
		// the panel's own startup sequence. So a live import left the accounts table
		// exactly as it was, and the Clients page (which reads that table and
		// nothing else, see ListAccounts) showed NOTHING until somebody happened to
		// restart the panel. The Inbounds page looked fine throughout, because it
		// reads settings.clients directly, which made it read as "the import lost my
		// customers" rather than as a page that had not been told about them.
		//
		// Both are additive and idempotent, and both are guarded against re-running,
		// so doing them here costs a no-op on the next start rather than a second
		// pass. Ordered as the startup sequence orders them: slots and subIds first
		// (MigrateDB has just done those), then the backfill that reads them.
		accountService := &AccountService{}
		accountService.MigrationAccounts()
		if err := (&GroupService{}).Migrate(); err != nil {
			return rollback(fmt.Errorf("preparing restored accounts and groups: %w", err))
		}
		// Imported wireguard inbounds get their clients array and their device
		// address here for the same reason: until they have one they cannot be
		// offered as somewhere to put an account.
		ReconcileAllWireguardXrayKeys()

		// Re-normalize tunnelled-protocol ranges for this host and bring the data
		// plane up against the imported inbounds.
		for _, proto := range vpnRangeProtocols {
			AutoExpandVpnRanges(proto)
		}
		s.l2tpService.InitL2tp()
		s.pptpService.InitPptp()
		if err = s.RestartXrayService(); err != nil {
			return rollback(fmt.Errorf("starting restored Xray: %w", err))
		}
	}

	if err := os.Remove(fallbackPath); err != nil {
		logger.Warning("import: failed to remove fallback file: ", err)
	}
	return report, nil
}

// snapshotPreservedSettings reads the effective value of each preserved key: the
// stored row if present, else the built-in default. A key with neither (e.g.
// radiusSecret on a never-provisioned box) is simply absent from the snapshot and
// is left as the backup has it.
func (s *ServerService) snapshotPreservedSettings() (map[string]string, error) {
	db := database.GetDB()
	var rows []model.Setting
	if err := db.Where("key IN ?", preservedSettingKeys).Find(&rows).Error; err != nil {
		return nil, err
	}
	stored := make(map[string]string, len(rows))
	for _, r := range rows {
		stored[r.Key] = r.Value
	}
	out := make(map[string]string, len(preservedSettingKeys))
	for _, key := range preservedSettingKeys {
		if v, ok := stored[key]; ok {
			out[key] = v
		} else if v, ok := defaultValueMap[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

// restorePreservedSettings upserts the snapshot into the freshly imported DB.
func (s *ServerService) restorePreservedSettings(preserved map[string]string) error {
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		for key, value := range preserved {
			setting := &model.Setting{}
			err := tx.Where("key = ?", key).First(setting).Error
			switch {
			case err == nil:
				setting.Value = value
				if err := tx.Save(setting).Error; err != nil {
					return err
				}
			case database.IsNotFound(err):
				if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
}

func (s *ServerService) buildImportReport(preserved map[string]string) *ImportReport {
	db := database.GetDB()
	report := &ImportReport{PreservedSettings: make([]string, 0, len(preserved))}
	// Report in the stable declared order rather than map order.
	for _, key := range preservedSettingKeys {
		if _, ok := preserved[key]; ok {
			report.PreservedSettings = append(report.PreservedSettings, key)
		}
	}
	var n int64
	if err := db.Model(&model.Inbound{}).Count(&n).Error; err == nil {
		report.Inbounds = int(n)
	}
	if err := db.Model(&xray.ClientTraffic{}).Count(&n).Error; err == nil {
		report.Clients = int(n)
	}
	if err := db.Model(&model.User{}).Count(&n).Error; err == nil {
		report.Admins = int(n)
	}
	return report
}
