// GOMailSync
// Copyright (C) 2014 Simone Gotti <simone.gotti@gmail.com>
//
// This program is free software; you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program; if not, write to the Free Software Foundation, Inc.,
// 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301 USA.

package mailsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
)

type Syncgroup struct {
	globalconfig *config.Config
	config       *config.SyncgroupConfig
	name         string
	metadatadir  string
	stores       []StoreManager
	logger       *log.Logger
	e            *errors.Error
	dryrun       bool
}

func (s *Syncgroup) newStore(globalconfig *config.Config, config *config.StoreConfig) (m StoreManager, err error) {
	basemetadatadir := filepath.Join(globalconfig.Metadatadir, "stores")

	// Disable expunge if deletemode == flag
	if s.config.Deletemode == "flag" {
		config.Expunge = false
	}

	switch config.StoreType {
	case "Maildir":
		m, err = NewMaildirStore(globalconfig, config, basemetadatadir, s.dryrun)
	case "IMAP":
		m, err = NewImapStore(globalconfig, config, basemetadatadir, s.dryrun)
	}
	return m, err
}

func mergeFolders(folders1 []Mailfolder, folders2 []Mailfolder, ignoreexcluded bool) []Mailfolder {
	fm := make(map[string]Mailfolder, 0)

	for _, f := range folders1 {
		fm[f.String()] = f
	}

	for _, f2 := range folders2 {
		if _, ok := fm[f2.String()]; ok {
			if f2.Excluded {
				f1 := fm[f2.String()]
				f1.Excluded = true
				fm[f1.String()] = f1
			}
		} else {
			fm[f2.String()] = f2
		}
	}

	// Remove excluded folders
	efolders := make([]Mailfolder, 0)
	for _, f := range fm {
		if !(ignoreexcluded && f.Excluded) {
			efolders = append(efolders, f)
		}
	}
	return efolders
}

func foldersLen(folders []Mailfolder, ignoreexcluded bool) (count int) {
	count = 0
	if !ignoreexcluded {
		return len(folders)
	} else {
		for _, f := range folders {
			if !f.Excluded {
				count++
			}
		}
	}
	return
}

func removeIgnoredMessages(messages []uint32, fm MailfolderManager) []uint32 {
	filteredmessages := make([]uint32, 0)
	for _, m := range messages {
		if !fm.IsIgnored(m) {
			filteredmessages = append(filteredmessages, m)
		}
	}
	return filteredmessages
}

func (s *Syncgroup) getSyncFolders() (folders []Mailfolder, err error) {
	store1 := s.stores[0]
	store2 := s.stores[1]

	folders1 := store1.GetFolders()
	folders2 := store2.GetFolders()

	folders = mergeFolders(folders1, folders2, true)

	return folders, nil
}

func NewSyncgroup(globalconfig *config.Config, config *config.SyncgroupConfig, dryrun bool) (s *Syncgroup, err error) {
	name := config.Name
	logprefix := fmt.Sprintf("syncgroup: %s", name)
	errprefix := fmt.Sprintf("syncgroup: %s", name)
	logger := log.GetLogger(logprefix, globalconfig.LogLevel)
	e := errors.New(errprefix)

	metadatadir := filepath.Join(globalconfig.Metadatadir, "syncgroups", name)

	err = os.MkdirAll(metadatadir, 0777)
	if err != nil {
		return nil, e.E(err)
	}

	s = &Syncgroup{
		globalconfig: globalconfig,
		config:       config,
		name:         name,
		metadatadir:  metadatadir,
		stores:       make([]StoreManager, 0),
		logger:       logger,
		e:            e,
		dryrun:       dryrun,
	}

	var storenumber Storenumber = Store1
	for _, storename := range config.Stores {
		var ok bool = false
		for _, storeconf := range globalconfig.Stores {
			if storeconf.Name == storename {
				ok = true
				store, err := s.newStore(globalconfig, storeconf)
				if err != nil {
					return nil, e.E(err)
				}
				s.stores = append(s.stores, store)
			}
		}
		if ok == false {
			err = fmt.Errorf("Missing store definition for:", s)
			return nil, e.E(err)
		}
		storenumber++
	}
	return
}

func (s *Syncgroup) SyncWrapper(interactions int, out chan error) {
	err := s.Sync(interactions)
	out <- err
}

func (s *Syncgroup) Sync(interactions int) (err error) {

	folders, err := s.getSyncFolders()
	if err != nil {
		return s.e.E(err)
	}

	s.logger.Infof("folders: %s", folders)
	var maxconcurrentsyncs uint8
	if int(s.config.Concurrentsyncs) > len(folders) {
		maxconcurrentsyncs = uint8(len(folders))
	} else {
		if s.config.Concurrentsyncs < 1 {
			maxconcurrentsyncs = 1
		} else {
			maxconcurrentsyncs = s.config.Concurrentsyncs
		}
	}

	c := make(chan SyncResult)
	sched := make(chan bool, 1)
	usedfolders := make(map[int]bool)
	usedfolderslock := new(sync.Mutex)
	var folderindex int = 0
	syncscount := uint8(0)
	countmap := make(map[int]int)

	sched <- true
	for {
		select {
		case result := <-c:
			s.logger.Debugf("SyncFolderWrapper goroutine exited with result: %#v. usedfolders: %v", result, usedfolders)
			err := result.Error
			if err != nil {
				s.logger.Errorf("Sync of folder %s failed with error: %s", result.Folder, err)
			}
			syncscount--
			countmap[result.Folderindex]++
			sched <- true

			time.AfterFunc(s.config.SyncInterval.Duration, func() {
				usedfolderslock.Lock()
				usedfolders[result.Folderindex] = false
				usedfolderslock.Unlock()
				sched <- true
			})

		case <-sched:
			for syncscount < maxconcurrentsyncs {
				found := false
				for i := 0; i < len(folders); i++ {
					usedfolderslock.Lock()
					used := usedfolders[folderindex]
					usedfolderslock.Unlock()
					if !used {
						found = true
						break
					}

					folderindex++
					if folderindex >= len(folders) {
						folderindex = 0
					}
				}

				if !found {
					break
				}

				if folderindex >= 0 {
					usedfolderslock.Lock()
					s.logger.Debugf("Starting SyncFolderWrapper for folder: %v. usedfolders before: %v", folders[folderindex], usedfolders)
					go s.SyncFolderWrapper(folders[folderindex], folderindex, c)
					usedfolders[folderindex] = true
					syncscount++
					s.logger.Debugf("Started SyncFolderWrapper for folder: %v. usedfolders after: %v", folders[folderindex], usedfolders)
					usedfolderslock.Unlock()
				}
			}
		}

		if interactions > 0 {
			finished := true
			for _, c := range countmap {
				if c < interactions {
					finished = false
				}
			}
			if finished {
				break
			}
		}
	}
	return
}

type SyncResult struct {
	Error       error
	Folder      Mailfolder
	Folderindex int
}

func (s *Syncgroup) SyncFolderWrapper(folder Mailfolder, folderindex int, out chan SyncResult) {
	err := s.SyncFolder(folder)
	result := SyncResult{err, folder, folderindex}
	out <- result
}

func (s *Syncgroup) SyncFolder(folder Mailfolder) (err error) {
	logprefix := fmt.Sprintf("%s %s %s", "syncgroup", s.name, folder)
	errprefix := logprefix
	logger := log.GetLogger(logprefix, s.globalconfig.LogLevel)
	e := errors.New(errprefix)

	logger.Debug("Syncing folder: ", folder)

	store1 := s.stores[0]
	store2 := s.stores[1]

	syncstatus, err := NewUIDMapSyncstatus(s.globalconfig, s.config, s.metadatadir, folder.Name)
	if err != nil {
		return e.E(err)
	}
	defer syncstatus.Close()

	syncstatus.UpdateSyncstatus()

	folder1, err := store1.GetMailfolderManager(folder.Name)
	if err != nil {
		return e.E(err)
	}
	defer folder1.Close()

	folder2, err := store2.GetMailfolderManager(folder.Name)
	if err != nil {
		return e.E(err)
	}
	defer folder2.Close()

	err = folder1.UpdateMessageList()
	if err != nil {
		return e.E(err)
	}
	err = folder2.UpdateMessageList()
	if err != nil {
		return e.E(err)
	}

	srcstore := store1
	dststore := store2
	srcfolder := folder1
	dstfolder := folder2

	syncstatus.SetSrcstore(Store1)

	for i := 0; i < 2; i++ {
		logprefix := fmt.Sprintf("%s %s %s -> %s %s", "syncgroup", s.name, srcstore.Name(), dststore.Name(), folder)
		errprefix := logprefix
		logger := log.GetLogger(logprefix, s.globalconfig.LogLevel)
		e := errors.New(errprefix)

		newMessages, err := syncstatus.GetNewMessages(srcfolder)
		if err != nil {
			return e.E(err)
		}
		// Remove ignored messages
		newMessages = removeIgnoredMessages(newMessages, srcfolder)
		logger.Infof("There are %d new messages", len(newMessages))

		deletedMessages, err := syncstatus.GetDeletedMessages(srcfolder)
		if err != nil {
			return e.E(err)
		}
		// Remove ignored messages
		deletedMessages = removeIgnoredMessages(deletedMessages, srcfolder)
		logger.Infof("There are %d deleted messages", len(deletedMessages))

		changedMessages, err := syncstatus.GetChangedMessages(srcfolder)
		if err != nil {
			return e.E(err)
		}
		// Remove ignored messages
		changedMessages = removeIgnoredMessages(changedMessages, srcfolder)
		logger.Infof("There are %d changed messages", len(changedMessages))

		// logger.Debugf("New messages:")
		// for _, u := range newMessages {
		// 	logger.Debugf("%d", u)
		// }

		if s.dryrun {
			continue
		}

		// Add new messages
		for _, srcuid := range newMessages {
			logger.Infof("Adding message with srcuid: %d to destination store: %s", srcuid, dststore.Name())

			syncstatus.BeginTx()

			body, err := srcfolder.ReadMessage(srcuid)
			if err != nil {
				syncstatus.Rollback()
				return e.E(err)
			}
			flags, err := srcfolder.GetFlags(srcuid)
			if err != nil {
				syncstatus.Rollback()
				return e.E(err)
			}

			dstuid, err := dstfolder.AddMessage(srcuid, flags, body)
			if err != nil {
				err := fmt.Errorf("AddMessage error: %s", err)
				syncstatus.Rollback()
				return e.E(err)
			} else {
				logger.Debug("Received dstuid: ", dstuid)

				// Ask srcfolder if it wants to update its message
				srcuid, err = srcfolder.Update(srcuid)
				if err != nil {
					// TODO remove message from dstfolder if srcfolder.Update() failed?
					return e.E(err)
				}
				err = syncstatus.Update(srcuid, dstuid, flags)
				if err != nil {
					logger.Errorf("error: %s", err)
					syncstatus.Rollback()
					return e.E(err)
				}
			}
			err = syncstatus.Commit()
			if err != nil {
				return e.E(err)
			}
		}

		// Remove deleted messages
		if s.config.Deletemode == "none" {
			logger.Info("deletemode is none. Skipping message deletion")
		} else {
			for _, srcuid := range deletedMessages {
				syncstatus.BeginTx()

				dstuid, err := syncstatus.GetDststoreUID(srcuid)
				if err != nil {
					syncstatus.Rollback()
					return e.E(err)
				}

				logger.Debugf("Deleting message with dstuid: %d from destination store: %s", dstuid, dststore.Name())

				if s.config.Deletemode == "expunge" {
					logger.Debug("Real deleting message")
					err = dstfolder.DeleteMessage(dstuid)
				} else if s.config.Deletemode == "trash" {
					logger.Debug("Marking message ad Deleted")
					flags, err := dstfolder.GetFlags(dstuid)
					if err != nil {
						syncstatus.Rollback()
						return e.E(err)
					}
					err = dstfolder.SetFlags(dstuid, addFlags(flags, "T"))
				} else {
					err = fmt.Errorf("Bad syncgroup deletemode(This should never happen!!!): \"%s\"", s.config.Deletemode)
					syncstatus.Rollback()
					return e.E(err)
				}
				if err != nil {
					err := fmt.Errorf("Delete error: %s", err)
					syncstatus.Rollback()
					return e.E(err)
				} else {
					err = syncstatus.Delete(srcuid)
					if err != nil {
						syncstatus.Rollback()
						return e.E(err)
					}
				}
				err = syncstatus.Commit()
				if err != nil {
					return e.E(err)
				}
			}
		}

		// Update Flags
		for _, srcuid := range changedMessages {
			syncstatus.BeginTx()

			dstuid, err := syncstatus.GetDststoreUID(srcuid)
			if err != nil {
				syncstatus.Rollback()
				return e.E(err)
			}
			flags, err := srcfolder.GetFlags(srcuid)
			if err != nil {
				syncstatus.Rollback()
				return e.E(err)
			}

			logger.Debugf("Updating message flags to message with dstuid %d in destination store %s to flags: \"%s\"", dstuid, dststore.Name(), flags)

			if dstfolder.HasUID(dstuid) {
				logger.Debugf("Changing message with dstuid: %d", dstuid)
				err = dstfolder.SetFlags(dstuid, flags)
				if err != nil {
					err := fmt.Errorf("dstfolder.SetFlags error: %s", err)
					syncstatus.Rollback()
					return e.E(err)
				} else {
					err = syncstatus.Update(srcuid, dstuid, flags)
					if err != nil {
						syncstatus.Rollback()
						return e.E(err)
					}
				}
			}
			err = syncstatus.Commit()
			if err != nil {
				return e.E(err)
			}
		}

		srcfolder = folder2
		dstfolder = folder1
		srcstore = store2
		dststore = store1
		syncstatus.SetSrcstore(Store2)
	}
	return
}

func (s *Syncgroup) List() (err error) {
	fmt.Printf("Syncgroup: %s\n", s.name)
	for _, store := range s.stores {
		fmt.Printf("\t")
		fmt.Printf("Store: %s\n", store.Name())

		folders := store.GetFolders()
		if err != nil {
			return s.e.E(err)
		}

		for _, folder := range folders {
			fmt.Printf("\t\t")
			fmt.Printf("%s ", folder)
			if folder.Excluded {
				fmt.Printf("(excluded)")
			}
			fmt.Printf("\n")
		}
	}

	fmt.Printf("\t")
	fmt.Printf("Will sync these folders:\n")

	folders, err := s.getSyncFolders()
	if err != nil {
		return s.e.E(err)
	}
	for _, folder := range folders {
		fmt.Printf("\t\t")
		fmt.Printf("%s\n", folder)
	}

	return
}
