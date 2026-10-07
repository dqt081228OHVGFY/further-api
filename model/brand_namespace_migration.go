package model

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// These names identify persisted data from earlier releases, not product
// branding. Keep this migration independent of source-code naming changes.
const retiredTablePrefix = "relay_for_"

// migrateBusinessNamespace runs before AutoMigrate. Renaming a Go model alone
// creates a new empty table, so balances, claims and channel policies must move
// with the namespace. Existing destination tables are never overwritten.
func migrateBusinessNamespace(db *gorm.DB) error {
	if db == nil {
		return errors.New("business namespace migration requires a database")
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return fmt.Errorf("inspect business tables: %w", err)
	}
	type tableMove struct{ source, target string }
	moves := make([]tableMove, 0)
	for _, table := range tables {
		if !strings.HasPrefix(table, retiredTablePrefix) {
			continue
		}
		target := "further_" + strings.TrimPrefix(table, retiredTablePrefix)
		if db.Migrator().HasTable(target) {
			return fmt.Errorf("business namespace conflict: both %s and %s exist; reconcile these tables before starting", table, target)
		}
		moves = append(moves, tableMove{source: table, target: target})
	}
	// Validate all destinations before changing any table. Each rename is
	// restartable even on databases whose DDL cannot be rolled back.
	for _, move := range moves {
		if err := db.Migrator().RenameTable(move.source, move.target); err != nil {
			return fmt.Errorf("rename business table %s: %w", move.source, err)
		}
	}
	return nil
}

// migrateBusinessValues runs after the current schema is present. It changes
// only recognized namespace values; user text and credentials remain intact.
func migrateBusinessValues(db *gorm.DB) error {
	if db == nil {
		return errors.New("business value migration requires a database")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&UserSubscription{}) {
			for _, suffix := range []string{"benefit_timed", "converted", "prepaid"} {
				if err := tx.Model(&UserSubscription{}).Where("source = ?", "relayfor_"+suffix).Update("source", "further_"+suffix).Error; err != nil {
					return fmt.Errorf("migrate subscription source: %w", err)
				}
			}
		}
		if !tx.Migrator().HasTable(&Option{}) {
			return nil
		}
		var options []Option
		if err := tx.Find(&options).Error; err != nil {
			return err
		}
		for _, option := range options {
			target := option.Key
			for _, prefix := range []struct{ old, current string }{{"RelayFor", "Further"}, {"relayfor", "further"}, {"RELAYFOR", "FURTHER"}} {
				if strings.HasPrefix(target, prefix.old) {
					target = prefix.current + strings.TrimPrefix(target, prefix.old)
					break
				}
			}
			if target == option.Key {
				continue
			}
			var existing Option
			err := tx.Where(&Option{Key: target}).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&Option{Key: target, Value: option.Value}).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			if err := tx.Delete(&Option{}, &Option{Key: option.Key}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
