package mailsync

type Syncstatus interface {
	SetSrcstore(store Storenumber)
	GetSrcstoreCol() (string, error)
	GetDststoreCol() (string, error)
	GetDststoreUID(srcuid uint32) (dstuid uint32, err error)
	HasUID(uid uint32) (bool, error)
	UpdateSyncstatus() error
	BeginTx() (err error)
	Commit() (err error)
	Rollback() (err error)
	Update(srcuid uint32, dstuid uint32, flags string) (err error)
	Delete(uid uint32) (err error)
	GetNewMessages(folder MailfolderManager) ([]uint32, error)
	GetDeletedMessages(folder MailfolderManager) ([]uint32, error)
	GetChangedMessages(folder MailfolderManager) ([]uint32, error)

	Close() (err error)
}
