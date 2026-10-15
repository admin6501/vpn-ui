package service

import (
	"fmt"
	"os"
	"sync"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
)

// Both import endpoints use the same database and staging paths.
var databaseImportMu sync.Mutex

// Close the rejected database before removing its WAL or putting the original
// file back. Keep the fallback on disk if replacement fails; never delete the
// only remaining copy of the previous database on an error path.
func (s *ServerService) rollbackDBRestore(fallbackPath string, cause error) error {
	if err := database.CloseDB(); err != nil {
		return fmt.Errorf("restore failed (%v); could not close rejected database (%v); previous database retained at %s", cause, err, fallbackPath)
	}
	dbPath := config.GetDBPath()
	database.RemoveSidecars(dbPath)
	if err := os.Rename(fallbackPath, dbPath); err != nil {
		return fmt.Errorf("restore failed (%v); could not restore previous database (%v); previous database retained at %s", cause, err, fallbackPath)
	}
	if err := database.InitDB(dbPath); err != nil {
		return fmt.Errorf("restore failed (%v); previous database is back on disk but could not be opened: %w", cause, err)
	}
	s.l2tpService.InitL2tp()
	s.pptpService.InitPptp()
	if err := s.RestartXrayService(); err != nil {
		return fmt.Errorf("restore failed (%v); previous database restored but Xray could not be restarted: %w", cause, err)
	}
	return fmt.Errorf("restore failed; previous database restored: %w", cause)
}
