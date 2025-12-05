package domain

// TODO: separate /application if we need extra separation on side-effects, use-cases, etc.
func NewApp(reader ProfileReadStore, writer ProfileWriteStore, signer CursorSigner) *Application {
	return &Application{
		reader: reader,
		writer: writer,
		signer: signer,
	}
}
