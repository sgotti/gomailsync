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
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"path/filepath"
	"sort"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
	"os"
)

type UIDMapSyncstatus struct {
	metadatadir string
	folder      *Mailfolder
	StatusDB    *sql.DB
	activeTx    *sql.Tx
	srcstore    Storenumber
	logger      *log.Logger
	e           *errors.Error
}

type Storenumber int

const (
	Store1 Storenumber = iota
	Store2
)

type Uint32Slice []uint32

func (p Uint32Slice) Len() int           { return len(p) }
func (p Uint32Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Uint32Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func NewUIDMapSyncstatus(globalconfig *config.Config, config *config.SyncgroupConfig, basemetadatadir string, folder *Mailfolder) (u Syncstatus, err error) {
	logprefix := fmt.Sprintf("%s %s %s", "uidmapsyncstatus", config.Name, folder)
	errprefix := logprefix
	logger := log.GetLogger(logprefix, globalconfig.LogLevel)
	e := errors.New(errprefix)

	metadatadir := filepath.Join(basemetadatadir, "uidmapsyncstatus", FolderToStorePath(folder, os.PathSeparator))
	err = os.MkdirAll(metadatadir, 0777)
	if err != nil {
		return nil, e.E(err)
	}

	statusdbfilepath := filepath.Join(metadatadir, "syncstatus.db")

	db, err := sql.Open("sqlite3", statusdbfilepath)
	if err != nil {
		return nil, e.E(err)
	}

	sql := `create table if not exists syncstatus (uidstore1 integer not null, uidstore2 integer not null, flags text, primary key (uidstore1, uidstore2));`

	_, err = db.Exec(sql)
	if err != nil {
		logger.Printf("%q: %s\n", err, sql)
		return nil, e.E(err)
	}
	u = &UIDMapSyncstatus{
		metadatadir: metadatadir,
		folder:      folder,
		StatusDB:    db,
		logger:      logger,
		e:           e,
	}

	return
}

func (u *UIDMapSyncstatus) Close() (err error) {
	u.StatusDB.Close()
	return
}

func (u *UIDMapSyncstatus) SetSrcstore(store Storenumber) {
	u.srcstore = store
}

func (u *UIDMapSyncstatus) GetSrcstoreCol() (string, error) {
	switch u.srcstore {
	case Store1:
		return "uidstore1", nil
	case Store2:
		return "uidstore2", nil
	}

	err := fmt.Errorf("Wrong srcstore")
	return "", err
}

func (u *UIDMapSyncstatus) GetDststoreCol() (string, error) {
	switch u.srcstore {
	case Store1:
		return "uidstore2", nil
	case Store2:
		return "uidstore1", nil
	}

	err := fmt.Errorf("Wrong srcstore")
	return "", err
}

func (u *UIDMapSyncstatus) GetDststoreUID(srcuid uint32) (dstuid uint32, err error) {
	srcuidcol, err := u.GetSrcstoreCol()
	dstuidcol, err := u.GetDststoreCol()
	if err != nil {
		return 0, u.e.E(err)
	}

	db := u.StatusDB

	query := fmt.Sprintf("select %s from syncstatus where %s = %d", dstuidcol, srcuidcol, srcuid)

	//u.log.Debug("query:", query)
	rows, err := db.Query(query)
	if err != nil {
		return 0, u.e.E(err)
	}
	defer rows.Close()

	for rows.Next() {
		rows.Scan(&dstuid)
		return
	}

	return
}

func (u *UIDMapSyncstatus) HasUID(uid uint32) (bool, error) {
	uidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return false, u.e.E(err)
	}

	db := u.StatusDB

	query := fmt.Sprintf("select %s, flags from syncstatus where %s = %d", uidcol, uidcol, uid)
	//u.log.Debug("query:", query)
	rows, err := db.Query(query)
	if err != nil {
		return false, u.e.E(err)
	}
	defer rows.Close()
	for rows.Next() {
		return true, nil
	}

	return false, nil
}

func (u *UIDMapSyncstatus) UpdateSyncstatus() error {
	return nil
}

func (u *UIDMapSyncstatus) BeginTx() (err error) {
	db := u.StatusDB

	tx, err := db.Begin()
	if err != nil {
		return u.e.E(err)
	}

	u.activeTx = tx

	return
}

func (u *UIDMapSyncstatus) Commit() (err error) {
	err = u.activeTx.Commit()
	if err != nil {
		return u.e.E(err)
	}
	u.activeTx = nil
	return
}

func (u *UIDMapSyncstatus) Rollback() (err error) {
	err = u.activeTx.Rollback()
	if err != nil {
		return u.e.E(err)
	}
	u.activeTx = nil
	return
}

func (u *UIDMapSyncstatus) Update(srcuid uint32, dstuid uint32, flags string) (err error) {
	srcuidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return u.e.E(err)
	}
	dstuidcol, err := u.GetDststoreCol()
	if err != nil {
		return u.e.E(err)
	}

	query := fmt.Sprintf("insert or replace into syncstatus(%s, %s, flags) values (?, ?, ?)", srcuidcol, dstuidcol)
	//u.log.Debug("query:", query)

	stmt, err := u.activeTx.Prepare(query)
	if err != nil {
		return u.e.E(err)
	}

	defer stmt.Close()
	_, err = stmt.Exec(srcuid, dstuid, flags)
	if err != nil {
		return u.e.E(err)
	}

	return u.e.E(err)
}

func (u *UIDMapSyncstatus) Delete(uid uint32) (err error) {
	srcuidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return u.e.E(err)
	}

	query := fmt.Sprintf("delete from syncstatus where %s = ?", srcuidcol)
	//u.log.Debug("query:", query)
	stmt, err := u.activeTx.Prepare(query)
	if err != nil {
		return u.e.E(err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(uid)
	if err != nil {
		return u.e.E(err)
	}

	return
}

func (u *UIDMapSyncstatus) GetNewMessages(folder MailfolderManager) ([]uint32, error) {

	messages := folder.GetMessages()

	db := u.StatusDB

	dstuidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return nil, u.e.E(err)
	}

	query := fmt.Sprintf("select %s, flags from syncstatus", dstuidcol)
	rows, err := db.Query(query)
	if err != nil {
		return nil, u.e.E(err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid uint32
		var flags string
		rows.Scan(&uid, &flags)

		delete(messages, uid)
	}

	newMessages := make([]uint32, 0)

	// Order the uids
	for uid, _ := range messages {
		newMessages = append(newMessages, uid)
	}
	// Order the uids
	sort.Sort(Uint32Slice(newMessages))
	return newMessages, nil
}

func (u *UIDMapSyncstatus) GetDeletedMessages(folder MailfolderManager) ([]uint32, error) {
	deletedMessages := make([]uint32, 0)

	db := u.StatusDB

	dstuidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return nil, u.e.E(err)
	}

	query := fmt.Sprintf("select %s, flags from syncstatus", dstuidcol)
	rows, err := db.Query(query)
	if err != nil {
		return nil, u.e.E(err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid uint32
		var flags string
		rows.Scan(&uid, &flags)

		if !folder.HasUID(uid) {
			u.logger.Debugf("uid %d not found", uid)
			deletedMessages = append(deletedMessages, uid)
		}
	}

	// Order the uids
	sort.Sort(Uint32Slice(deletedMessages))
	return deletedMessages, nil
}

func (u *UIDMapSyncstatus) GetChangedMessages(folder MailfolderManager) ([]uint32, error) {
	changedMessages := make([]uint32, 0)

	db := u.StatusDB

	dstuidcol, err := u.GetSrcstoreCol()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("select %s, flags from syncstatus", dstuidcol)
	rows, err := db.Query(query)
	if err != nil {
		return nil, u.e.E(err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid uint32
		var flags string
		rows.Scan(&uid, &flags)

		if folder.HasUID(uid) {
			messageflags, err := folder.GetFlags(uid)
			if err != nil {
				return nil, u.e.E(err)
			}
			if flags != messageflags {
				changedMessages = append(changedMessages, uid)
			}
		}
	}

	// Order the uids
	sort.Sort(Uint32Slice(changedMessages))
	return changedMessages, nil
}
