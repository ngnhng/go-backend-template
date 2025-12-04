package domain

import "app/modules/db"

// TODO: separate /application if we need extra separation on side-effects, use-cases, etc.
func NewApp(pool db.ConnectionPool, persistence ProfilePersistence, signer CursorSigner) *Application {
	return &Application{pool: pool, persistence: persistence, signer: signer}
}
