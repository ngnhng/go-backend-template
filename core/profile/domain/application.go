package domain

import "app/db"

func NewApp(pool db.ConnectionPool, persistence ProfilePersistence, signer CursorSigner) *Application {
	return &Application{pool: pool, persistence: persistence, signer: signer}
}
