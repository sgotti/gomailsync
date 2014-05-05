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
	"os"
	"path/filepath"
	"testing"

	"github.com/sgotti/gomailsync/config"
)

var maildirfoldertest struct {
	globalconfig   *config.Config
	syncgroup1conf *config.SyncgroupConfig

	store1 StoreManager
	store2 StoreManager
	fm1    MailfolderManager
}

func SetupMaildirFolderTest(t *testing.T) {

	testdir, _ := ioutil.TempDir("", "gomailsync-tests-")
	metadatadir := filepath.Join(testdir, "metadatadir")
	os.Mkdir(metadatadir, 0777)
	maildirstore1dir := filepath.Join(testdir, "maildirstore1")
	os.Mkdir(maildirstore1dir, 0777)
	maildirstore2dir := filepath.Join(testdir, "maildirstore2")
	os.Mkdir(maildirstore2dir, 0777)

	store1conf := config.StoreConfig{
		Name:       "store1",
		StoreType:  "Maildir",
		Maildir:    maildirstore1dir,
		Separator:  os.PathSeparator,
		UIDMapping: "files",
	}

	store2conf := config.StoreConfig{
		Name:       "store2",
		StoreType:  "Maildir",
		Maildir:    maildirstore2dir,
		Separator:  os.PathSeparator,
		UIDMapping: "files",
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
	}

	store1, _ := newStore(&globalconfig, &store1conf)

	folder := &Mailfolder{[]string{"INBOX"}, false}
	err := store1.CreateFolder(folder.Name)
	if err != nil {
		t.Fatal(err)
	}

	tmpfm, err := store1.GetMailfolderManager(folder.Name)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 0)
	for i := 0; i < 10; i++ {
		tmpfm.AddMessage(uint32(i), "", data)
	}

	fm1, _ := store1.GetMailfolderManager(folder.Name)

	maildirfoldertest.store1 = store1
	maildirfoldertest.fm1 = fm1

	maildirfoldertest.globalconfig = &globalconfig
	maildirfoldertest.syncgroup1conf = &syncgroup1conf

}

func TestSplitFilename(t *testing.T) {
	SetupMaildirFolderTest(t)
	fm1, _ := maildirfoldertest.fm1.(*MaildirFolder)

	exfilename := "1397565555_19.22053.localhost.localdomain,u=19,f=35745cb548222dd3d38d87c3deb395c2"
	exflags := ""
	filename, flags, err := fm1.splitFilename(exfilename + string(fm1.infoSeparator) + "2," + exflags)
	if filename != exfilename || flags != exflags || err != nil {
		t.Fatalf("Expected filename \"%s\", found \"%s\". Expected flags  \"%s\", found \"%s\". Error: %s", exfilename, filename, exflags, flags, err)
	}

	exfilename = "1397565555_19.22053.localhost.localdomain,u=19,f=35745cb548222dd3d38d87c3deb395c2"
	exflags = "ST"
	filename, flags, err = fm1.splitFilename(exfilename + string(fm1.infoSeparator) + "2," + exflags)
	if filename != exfilename || flags != exflags || err != nil {
		t.Fatalf("Expected filename \"%s\", found \"%s\". Expected flags  \"%s\", found \"%s\". Error: %s", exfilename, filename, exflags, flags, err)
	}

	filename, flags, err = fm1.splitFilename("abcdefghijklmnopqrstuvwxyz:123456OA")
	if err == nil {
		t.Fatalf("Expected Error != nil. found err: %s. Filename: \"%s\". Flags: \"%s\".", err, exfilename, exflags)
	}

	filename, flags, err = fm1.splitFilename("abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatalf("Expected Error != nil. found err: %s. Filename: \"%s\". Flags: \"%s\".", err, exfilename, exflags)
	}
}

func TestUpdateMessageList(t *testing.T) {
	SetupMaildirFolderTest(t)
	fm1, _ := maildirfoldertest.fm1.(*MaildirFolder)

	folder := Mailfolder{[]string{"INBOX"}, false}

	var startuid uint32 = 100000
	expected := 10
	addMessage(t, maildirfoldertest.store1, folder, "file02:2,S", "cur")
	expected++
	err := fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

	fullfilename, err := fm1.generateFullFilename(startuid, "S")
	addMessage(t, maildirfoldertest.store1, folder, fullfilename, "cur")
	expected++
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

	fullfilename, err = fm1.generateFullFilename(startuid, "S")
	addMessage(t, maildirfoldertest.store1, folder, fullfilename, "cur")
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)
	if fm1.messages[uint32(startuid)].Ignore == false {
		t.Fatalf("Expected message with uid %d as Ignored. Found: %#v", startuid, fm1.messages[uint32(startuid)])
	}

	addMessage(t, maildirfoldertest.store1, folder, "file03:wrongwrong", "cur")
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

	addMessage(t, maildirfoldertest.store1, folder, "file03:wrongwrong", "new")
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

	addMessage(t, maildirfoldertest.store1, folder, "file03withoutinfoseparator", "new")
	expected++
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

	fullfilename = "1397565555_19.22053.localhost.localdomain,u=19,f=thisfolderuiddoesntexists:2,ST"
	addMessage(t, maildirfoldertest.store1, folder, fullfilename, "cur")
	expected++
	err = fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	countMessages(t, maildirfoldertest.store1, folder, expected)

}

func TestDeleteMessage(t *testing.T) {
	SetupMaildirFolderTest(t)
	fm1 := maildirfoldertest.fm1

	folder := Mailfolder{[]string{"INBOX"}, false}

	err := fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	err = fm1.DeleteMessage(1)
	if err != nil {
		t.Fatal(err)
	}

	countMessages(t, maildirfoldertest.store1, folder, 9)

	err = fm1.DeleteMessage(100000)
	if err == nil {
		t.Fatal("Message doesn't exists. Deletemessage should return an errorr")
	}
	countMessages(t, maildirfoldertest.store1, folder, 9)

	fm1.Close()
}

func TestAddMessages(t *testing.T) {
	SetupMaildirFolderTest(t)
	fm1 := maildirfoldertest.fm1

	folder := Mailfolder{[]string{"INBOX"}, false}

	err := fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 0)

	_, err = fm1.AddMessage(uint32(0), "", data)
	if err != nil {
		t.Fatal(err)
	}

	countMessages(t, maildirfoldertest.store1, folder, 11)

	fm1.Close()
}

func TestSetFlags(t *testing.T) {
	SetupMaildirFolderTest(t)
	fm1 := maildirfoldertest.fm1

	folder := Mailfolder{[]string{"INBOX"}, false}

	err := fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	err = fm1.SetFlags(uint32(0), "S")
	if err != nil {
		t.Fatal(err)
	}

	countMessages(t, maildirfoldertest.store1, folder, 10)

	flags, err := fm1.GetFlags(uint32(0))
	if flags != "S" {
		t.Fatal(err)
	}

	fm1.Close()
}

func countMessages(t *testing.T, store StoreManager, folder Mailfolder, expected int) (err error) {
	fm, _ := store.GetMailfolderManager(folder.Name)
	defer fm.Close()
	err = fm.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	messages := fm.GetMessages()
	if len(messages) != expected {
		t.Fatalf("Wrong number of messages: %d, expected: %d", len(messages), expected)
	}
	return
}

func newStore(globalconfig *config.Config, config *config.StoreConfig) (m StoreManager, err error) {
	basemetadatadir := filepath.Join(globalconfig.Metadatadir, "stores")

	switch config.StoreType {
	case "Maildir":
		m, err = NewMaildirStore(globalconfig, config, basemetadatadir, false)
	case "IMAP":
		m, err = NewImapStore(globalconfig, config, basemetadatadir, false)
	}
	return m, err
}
