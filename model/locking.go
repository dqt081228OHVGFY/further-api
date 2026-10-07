package model

import (
	"errors"
	"github.com/dqt081228OHVGFY/further-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockForUpdate makes the next query emit SELECT ... FOR UPDATE so the matched
// rows stay locked until the surrounding transaction ends.
//
// GORM v2 silently ignores the legacy `Set("gorm:query_option", "FOR UPDATE")`
// from GORM v1, so that form does not lock anything. Always use this helper
// instead.
//
// SQLite has no FOR UPDATE syntax (the clause would be a syntax error), so it
// is skipped there; SQLite's single-writer model makes one of two conflicting
// transactions fail instead of both committing.
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// SQLite's deferred transactions cannot wait while upgrading a shared read
// lock to a writer when another writer is waiting on that same read lock.
// Acquire its writer reservation before reading authorization facts. The
// statement preserves all business fields and honors the transaction context.
func reserveAccountMutationWriter(tx *gorm.DB, userID int) error {
	if tx == nil || userID <= 0 {
		return errors.New("账户事务参数无效。")
	}
	if tx.Dialector.Name() != "sqlite" {
		return nil
	}
	return tx.Model(&User{}).Where("id = ?", userID).
		UpdateColumn("auth_version", gorm.Expr("auth_version")).Error
}
