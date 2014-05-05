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

	"fmt"
	"github.com/sgotti/gomailsync/config"
	"reflect"
)

var synccgrouptest struct {
	globalconfig   *config.Config
	syncgroup1conf *config.SyncgroupConfig

	store1         StoreManager
	store2         StoreManager
	foldermanager1 MailfolderManager
}

func SetupSyncgroupTest(t *testing.T) {

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
		Separator:  '.',
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
	store2, _ := newStore(&globalconfig, &store2conf)

	folder := Mailfolder{[]string{"dir01", "child01"}, false}
	store1.CreateFolder(folder.Name)

	tmpfoldermanager, err := store1.GetMailfolderManager(folder.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfoldermanager.Close()

	data := make([]byte, 0)
	for i := 0; i < 10; i++ {
		tmpfoldermanager.AddMessage(uint32(i), "", data)
	}
	for i := 0; i < 10; i++ {
		tmpfoldermanager.AddMessage(uint32(i), "S", data)
	}

	foldermanager1, err := store1.GetMailfolderManager(folder.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer foldermanager1.Close()

	synccgrouptest.store1 = store1
	synccgrouptest.store2 = store2
	synccgrouptest.foldermanager1 = foldermanager1

	synccgrouptest.globalconfig = &globalconfig
	synccgrouptest.syncgroup1conf = &syncgroup1conf
}

func TestSyncgroupSync(t *testing.T) {

	SetupSyncgroupTest(t)

	syncgroup, err := NewSyncgroup(synccgrouptest.globalconfig, synccgrouptest.syncgroup1conf, false)
	if err != nil {
		t.Fatal(err)
	}
	store1 := syncgroup.stores[0]
	store2 := syncgroup.stores[1]

	folder := Mailfolder{[]string{"dir01", "child01"}, false}
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}

	expected := 20
	verifySync(t, syncgroup, folder, expected)

	// Remove one message from store1 dir01/child01 folder with empty flags
	removeMessage(t, store1, folder, getExistingUID(t, store1, folder, ""))
	expected--
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Remove one message from store2 dir01/child01 folder with empty flags
	removeMessage(t, store2, folder, getExistingUID(t, store2, folder, ""))
	expected--
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Remove one message from store1 dir01/child01 folder with flags "S"
	removeMessage(t, store1, folder, getExistingUID(t, store1, folder, "S"))
	expected--
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Remove one message from store2 dir01/child01 folder with flags "S"
	removeMessage(t, store2, folder, getExistingUID(t, store2, folder, "S"))
	expected--
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	setFlags(t, store2, folder, getExistingUID(t, store2, folder, "S"), "T")
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	setFlags(t, store1, folder, getExistingUID(t, store1, folder, ""), "D")
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Add some new valid messages in folder
	addMessage(t, store1, folder, "file01", "new")
	expected++
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Add some new valid messages in folder
	addMessage(t, store2, folder, "file02:2,S", "cur")
	expected++
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Another sync
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Test deletemode = trash

	syncgroup.config.Deletemode = "trash"
	// Remove one message from store2 dir01/child01 folder with flags "S"
	removeMessage(t, store1, folder, getExistingUID(t, store1, folder, "S"))
	// The number of expected messages should be the same as before. The deleted message should be redownload by foldermanager2.
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

	// Remove one message from store2 dir01/child01 folder with flags "S"
	removeMessage(t, store2, folder, getExistingUID(t, store2, folder, "S"))
	// The number of expected messages in foldermanager2 should be one less then store1.
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}

	countMessages(t, store1, folder, expected)
	countMessages(t, store2, folder, expected-1)

	// Doing another sync will redownload the deleted message from foldermanager1 to foldermanager2
	err = syncgroup.SyncFolder(folder)
	if err != nil {
		t.Fatal(err)
	}
	verifySync(t, syncgroup, folder, expected)

}

func TestSyncgroupMergeFolders(t *testing.T) {
	fs1 := []Mailfolder{}
	fs2 := []Mailfolder{}

	ef1 := Mailfolder{Name: []string{"folder01"}}
	ef2 := Mailfolder{Name: []string{"folder02"}}
	ef3 := Mailfolder{Name: []string{"folder03"}}
	ef4 := Mailfolder{Name: []string{"folder04"}}
	ef5 := Mailfolder{Name: []string{"folder05"}}
	ef6 := Mailfolder{Name: []string{"folder06"}}

	emfs := []Mailfolder{}
	mfs := mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder01"}})
	emfs = []Mailfolder{ef1}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder02"}})
	emfs = []Mailfolder{ef1, ef2}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs2 = append(fs2, Mailfolder{Name: []string{"folder02"}})
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs2 = append(fs2, Mailfolder{Name: []string{"folder03"}})
	emfs = []Mailfolder{ef1, ef2, ef3}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder03"}})
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder04"}, Excluded: true})
	ef4.Excluded = true
	emfs = []Mailfolder{ef1, ef2, ef3, ef4}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	emfs = []Mailfolder{ef1, ef2, ef3}
	mfs = mergeFolders(fs1, fs2, true)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder05"}, Excluded: false})
	fs2 = append(fs2, Mailfolder{Name: []string{"folder05"}, Excluded: true})
	ef5.Excluded = true
	emfs = []Mailfolder{ef1, ef2, ef3, ef4, ef5}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	emfs = []Mailfolder{ef1, ef2, ef3}
	mfs = mergeFolders(fs1, fs2, true)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	fs1 = append(fs1, Mailfolder{Name: []string{"folder06"}})
	emfs = []Mailfolder{ef1, ef2, ef3, ef4, ef5, ef6}
	mfs = mergeFolders(fs1, fs2, false)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}

	emfs = []Mailfolder{ef1, ef2, ef3, ef6}
	mfs = mergeFolders(fs1, fs2, true)
	if !checkMergedFolders(emfs, mfs) {
		t.Fatalf("Expecting folders %v, found %v", emfs, mfs)
	}
}

func addMessage(t *testing.T, store StoreManager, folder Mailfolder, name string, subdir string) {
	foldermanager, _ := store.GetMailfolderManager(folder.Name)
	var maildirfoldermanager *MaildirFolder = foldermanager.(*MaildirFolder)
	err := foldermanager.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	defer foldermanager.Close()

	filepath := filepath.Join(maildirfoldermanager.maildir, subdir, name)
	fo, err := os.Create(filepath)
	if err != nil {
		t.Fatal(err)
	}
	defer fo.Close()
}

func removeMessage(t *testing.T, store StoreManager, folder Mailfolder, uid uint32) {
	foldermanager, _ := store.GetMailfolderManager(folder.Name)
	err := foldermanager.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	defer foldermanager.Close()

	err = foldermanager.DeleteMessage(uid)
	if err != nil {
		t.Fatal(err)
	}
}

func setFlags(t *testing.T, store StoreManager, folder Mailfolder, uid uint32, flags string) {
	foldermanager, _ := store.GetMailfolderManager(folder.Name)
	err := foldermanager.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	defer foldermanager.Close()

	err = foldermanager.SetFlags(uid, flags)
	if err != nil {
		t.Fatal(err)
	}
}

func getExistingUID(t *testing.T, store StoreManager, folder Mailfolder, flags string) uint32 {
	foldermanager, _ := store.GetMailfolderManager(folder.Name)
	err := foldermanager.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	defer foldermanager.Close()

	for _, m := range foldermanager.GetMessages() {
		if m.Flags == flags {
			return m.UID
		}
	}

	t.Fatalf("No messages in folder with flags: %s", flags)
	return 0
}

func verifySync(t *testing.T, syncgroup *Syncgroup, folder Mailfolder, expected int) {
	store1 := syncgroup.stores[0]
	store2 := syncgroup.stores[1]
	foldermanager1, _ := store1.GetMailfolderManager(folder.Name)
	foldermanager2, _ := store2.GetMailfolderManager(folder.Name)
	defer foldermanager1.Close()
	defer foldermanager2.Close()

	err := foldermanager1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}
	err = foldermanager2.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	countMessages(t, store1, folder, expected)
	countMessages(t, store2, folder, expected)

	// Verify flags
	syncstatus, err := NewUIDMapSyncstatus(synccgrouptest.globalconfig, synccgrouptest.syncgroup1conf, syncgroup.metadatadir, folder.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer syncstatus.Close()

	syncstatus.SetSrcstore(Store1)
	for u1, _ := range foldermanager1.GetMessages() {
		flags1, err := foldermanager1.GetFlags(u1)
		if err != nil {
			t.Fatal(err)
		}

		u2, err := syncstatus.GetDststoreUID(u1)
		if err != nil {
			t.Fatal(err)
		}

		flags2, err := foldermanager2.GetFlags(u2)
		if err != nil {
			t.Fatal(err)
		}

		if flags1 != flags2 {
			t.Fatalf("Wrong flags! message1 uid: %d flags: %s, message2 uid: %d flags: %s", u1, flags1, u2, flags2)
		}
	}
}

func checkMergedFolders(mf1 []Mailfolder, mf2 []Mailfolder) bool {
	fmt.Printf("mf1: %v, mf2: %v\n", mf1, mf2)
	if len(mf1) != len(mf2) {
		return false
	}
	mf1m := make(map[string]Mailfolder)
	mf2m := make(map[string]Mailfolder)
	for _, f := range mf1 {
		mf1m[f.String()] = f
	}
	for _, f := range mf2 {
		mf2m[f.String()] = f
	}
	return reflect.DeepEqual(mf1m, mf2m)
}
