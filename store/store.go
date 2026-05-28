package store

import (
	"database/sql"
	"sync"

	"github.com/devlopali-dev/slash/server/profile"
)

// Store provides database access to all raw objects.
type Store struct {
	profile *profile.Profile
	driver  Driver

	workspaceSettingCache sync.Map // map[string]*WorkspaceSetting
	userCache             sync.Map // map[int]*User
	userSettingCache      sync.Map // map[string]*UserSetting
	shortcutCache         sync.Map // map[int]*Shortcut
}

// New creates a new instance of Store.
func New(driver Driver, profile *profile.Profile) *Store {
	return &Store{
		driver:  driver,
		profile: profile,
	}
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.driver.Close()
}

// GetDB exposes the underlying *sql.DB for direct queries in tests.
func (s *Store) GetDB() *sql.DB {
	return s.driver.GetDB()
}
