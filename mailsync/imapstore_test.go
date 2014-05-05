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
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mxk/go-imap/imap"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/tests/imapmock"
)

func init() {
	imap.DefaultLogMask = imap.LogRaw
}

var imapstoretest struct {
	globalconfig   *config.Config
	syncgroup1conf *config.SyncgroupConfig

	s1  StoreManager
	s2  StoreManager
	fm1 MailfolderManager
	c1  *imap.Client

	server *imapmock.Server
	conn   *imapmock.Connection
}

func SetupImapStoreTest(t *testing.T) {
	server := imapmock.NewMockImapServer(t, "* PREAUTH [CAPABILITY IMAP4rev1 UNSELECT UIDPLUS] Server ready")
	saddr := server.GetServerAddress()
	shost, sportstr, _ := net.SplitHostPort(saddr.String())
	sport, _ := strconv.ParseUint(sportstr, 10, 16)
	testdir, _ := ioutil.TempDir("", "gomailsync-tests-")
	metadatadir := filepath.Join(testdir, "metadatadir")
	os.Mkdir(metadatadir, 0777)

	store1conf := config.StoreConfig{
		Name:      "store1",
		StoreType: "IMAP",
		Host:      shost,
		Port:      uint16(sport),
	}

	store2conf := config.StoreConfig{
		Name:      "store2",
		StoreType: "IMAP",
		Host:      shost,
		Port:      uint16(sport),
	}

	syncgroup1conf := config.SyncgroupConfig{
		Name:            "syncgroup1",
		Stores:          []string{"store1", "store2"},
		Concurrentsyncs: 4,
		Deletemode:      "expunge",
	}

	globalconfig := config.Config{
		Metadatadir: metadatadir,
		Syncgroups:  []*config.SyncgroupConfig{&syncgroup1conf},
		Stores:      []*config.StoreConfig{&store1conf, &store2conf},
		LogLevel:    "debug",
		DebugImap:   true,
	}

	ch := make(chan *imapmock.Connection, 1)
	go func(server *imapmock.Server, ch chan *imapmock.Connection) {
		conn, _ := server.WaitConnection()
		log.Println("conn:", conn)
		conn.Script(
			`C: TAG0 LIST "" "*"`,
			`S: * LIST (\HasChildren \Trash) "." Trash`,
			`S: * LIST (\HasChildren \Drafts) "." Drafts`,
			`S: * LIST (\HasNoChildren \Sent) "." Sent`,
			`S: * LIST (\HasNoChildren) "." INBOX.dir01`,
			`S: * LIST (\HasChildren \Noselect) "." dir01`,
			`S: * LIST (\HasNoChildren) "." dir01.dir01`,
			`S: * LIST (\HasChildren) "." INBOX`,
			`S: TAG0 OK LIST completed`,
		)
		conn.Check()
		ch <- conn
	}(server, ch)

	s1, err := newStore(&globalconfig, &store1conf)
	if err != nil {
		t.Fatal(err)
	}

	conn := <-ch
	imapstoretest.conn = conn

	imapstoretest.server = server
	imapstoretest.s1 = s1

	imapstoretest.globalconfig = &globalconfig
	imapstoretest.syncgroup1conf = &syncgroup1conf
}

func TestImapStoreUpdateFolder(t *testing.T) {
	SetupImapStoreTest(t)

	s1 := imapstoretest.s1
	folders := s1.GetFolders()

	expected := 6
	if len(folders) != expected {
		t.Fatalf("Expected %d folders, found %d", expected, len(folders))
	}

	NoSelectFolder := Mailfolder{[]string{"dir01"}, false}
	if containsFolder(t, folders, NoSelectFolder, false) {
		t.Fatalf("Folder %s, should be ignored as is of type \\Noselect", NoSelectFolder)
	}
}

func containsFolder(t *testing.T, folders []Mailfolder, folder Mailfolder, checkexcluded bool) bool {
	for _, f := range folders {
		okexcluded := (f.Excluded == folder.Excluded) || !checkexcluded
		if StrsEquals(f.Name, folder.Name) && okexcluded {
			return true
		}
	}
	return false
}
