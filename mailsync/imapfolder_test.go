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
	"reflect"
	"strconv"
	"testing"

	"github.com/mxk/go-imap/imap"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/tests/imapmock"
	"strings"
)

func init() {
	imap.DefaultLogMask = imap.LogRaw
}

var imapfoldertest struct {
	globalconfig   *config.Config
	syncgroup1conf *config.SyncgroupConfig

	s1  StoreManager
	s2  StoreManager
	fm1 MailfolderManager

	server *imapmock.Server
	conn   *imapmock.Connection
	connfm *imapmock.Connection
}

func SetupImapFolderTest(t *testing.T) {
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
	imapfoldertest.conn = conn

	folder := Mailfolder{[]string{"INBOX"}, false}

	conn.Script(
		`C: TAG0 EXAMINE "INBOX"`,
		`S: * FLAGS (\Answered \Flagged \Draft \Deleted \Seen $Phishing $Forwarded $label1 $MDNSent $has_cal Old receipt-handled NonJunk $NotPhishing Junk)`,
		`S: * OK [PERMANENTFLAGS ()] Flags permitted.`,
		`S: * OK [UIDVALIDITY 2] UIDs valid.`,
		`S: * 5773 EXISTS`,
		`S: * 0 RECENT`,
		`S: * OK [UIDNEXT 528661] Predicted next UID.`,
		`S: * OK [HIGHESTMODSEQ 22028640]`,
		`S: TAG0 OK [READ-ONLY] INBOX selected. (Success)`,
		`C: TAG1 UNSELECT`,
		`S: TAG1 OK Returned to authenticated state. (Success)`,
	)

	go func(server *imapmock.Server, ch chan *imapmock.Connection) {
		conn, _ := server.WaitConnection()
		log.Println("conn:", conn)
		conn.Script(
			`C: TAG0 SELECT "INBOX"`,
			`S: * FLAGS (\Answered \Flagged \Draft \Deleted \Seen $Phishing $Forwarded $label1 $MDNSent $has_cal Old receipt-handled NonJunk $NotPhishing Junk)`,
			`S: * OK [PERMANENTFLAGS ()] Flags permitted.`,
			`S: * OK [UIDVALIDITY 2] UIDs valid.`,
			`S: * 5773 EXISTS`,
			`S: * 0 RECENT`,
			`S: * OK [UIDNEXT 528661] Predicted next UID.`,
			`S: * OK [HIGHESTMODSEQ 22028640]`,
			`S: TAG0 OK [READ-WRITE] INBOX selected. (Success)`,
		)
		ch <- conn
	}(server, ch)

	fm1, err := s1.GetMailfolderManager(folder.Name)
	if err != nil {
		t.Fatal(err)
	}
	conn.Check()

	connfm := <-ch
	imapfoldertest.connfm = connfm

	imapfoldertest.server = server
	imapfoldertest.s1 = s1
	imapfoldertest.fm1 = fm1

	imapfoldertest.globalconfig = &globalconfig
	imapfoldertest.syncgroup1conf = &syncgroup1conf

}

func TestImapFolderUIDValidity(t *testing.T) {
	SetupImapFolderTest(t)

	s1 := imapfoldertest.s1
	fm1 := imapfoldertest.fm1

	conn := imapfoldertest.conn
	connfm := imapfoldertest.connfm

	connfm.Script(
		`C: TAG0 UNSELECT`,
		`S: TAG0 OK Returned to authenticated state. (Success)`,
	)
	err := fm1.Close()
	if err != nil {
		t.Fatal(err)
	}
	connfm.Check()

	folder := Mailfolder{[]string{"INBOX"}, false}

	conn.Script(
		`C: TAG0 EXAMINE "INBOX"`,
		`S: * FLAGS (\Answered \Flagged \Draft \Deleted \Seen $Phishing $Forwarded $label1 $MDNSent $has_cal Old receipt-handled NonJunk $NotPhishing Junk)`,
		`S: * OK [PERMANENTFLAGS ()] Flags permitted.`,
		`S: * OK [UIDVALIDITY 3] UIDs valid.`,
		`S: * 5773 EXISTS`,
		`S: * 0 RECENT`,
		`S: * OK [UIDNEXT 528661] Predicted next UID.`,
		`S: * OK [HIGHESTMODSEQ 22028640]`,
		`S: TAG0 OK [READ-ONLY] INBOX selected. (Success)`,
		`C: TAG1 UNSELECT`,
		`S: TAG1 OK Returned to authenticated state. (Success)`,
		func(s *imapmock.Server) error { return conn.Close() },
	)

	fm1, err = s1.GetMailfolderManager(folder.Name)

	log.Println("err:", err)
	if err == nil {
		t.Fatal("Expected wrong uidvalidity error")
	}
	if !strings.Contains(err.Error(), "doesn't match saved uidvalidity") {
		t.Fatal("Expected wrong uidvalidity error")
	}
	conn.Check()
}

func TestImapFolderUpdateMessageList(t *testing.T) {
	SetupImapFolderTest(t)

	fm1 := imapfoldertest.fm1
	connfm := imapfoldertest.connfm

	connfm.Script(`C: TAG0 UID FETCH 1:* (UID FLAGS)`,
		`S: * 1 FETCH (UID 1 FLAGS (\Seen))`,
		`S: * 2 FETCH (UID 2 FLAGS ())`,
		`S: * 3 FETCH (UID 4 FLAGS ())`,
		`S: * 8 FETCH (UID 12 FLAGS (\Deleted))`,
		`S: * 9 FETCH (UID 13 FLAGS (\Answered))`,
		`S: * 10 FETCH (UID 15 FLAGS (\Draft))`,
		`S: * 11 FETCH (UID 19 FLAGS (\Flagged))`,
		`S: * 12 FETCH (UID 31 FLAGS ())`,
		`S: * 13 FETCH (UID 24 FLAGS (\Seen))`,
		`S: * 14 FETCH (UID 26 FLAGS (\Seen \Answered))`,
		`S: * 15 FETCH (UID 27 FLAGS (\Seen \Draft))`,
		`S: * 16 FETCH (UID 29 FLAGS (\Seen \Flagged))`,
		`S: * 17 FETCH (UID 35 FLAGS (\Seen \Deleted))`,
		`S: * 18 FETCH (UID 41 FLAGS ())`,
		`S: TAG0 OK Fetch completed.`,
	)

	err := fm1.UpdateMessageList()
	if err != nil {
		t.Fatal(err)
	}

	connfm.Check()

	messages := fm1.GetMessages()
	expected := 14
	if len(messages) != expected {
		t.Fatalf("Expected %d messages, found %d", expected, len(messages))
	}
}

func TestImapFolderFlagsToString(t *testing.T) {
	flagset := imap.NewFlagSet()

	s := ImapFlagsToString(flagset)
	expected := ""
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\Seen`] = true
	s = ImapFlagsToString(flagset)
	expected = "S"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\Answered`] = true
	s = ImapFlagsToString(flagset)
	expected = "RS"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\Deleted`] = true
	s = ImapFlagsToString(flagset)
	expected = "RST"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\Draft`] = true
	s = ImapFlagsToString(flagset)
	expected = "DRST"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\Flagged`] = true
	s = ImapFlagsToString(flagset)
	expected = "DFRST"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

	flagset[`\AnotherFlag`] = true
	s = ImapFlagsToString(flagset)
	expected = "DFRST"
	if s != expected {
		t.Fatalf("Expecting flags \"%s\", found \"%s\"", expected, s)
	}

}

func TestImapFolderStringToImapFlags(t *testing.T) {
	s := ""
	flagset := StringToImapFlags(s)

	if len(flagset) != 0 {
		t.Fatalf("Expecting empty flagset")
	}

	s = "S"
	flagset = StringToImapFlags(s)
	expected := imap.NewFlagSet(`\Seen`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

	s = "RS"
	flagset = StringToImapFlags(s)
	expected = imap.NewFlagSet(`\Seen`, `\Answered`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

	s = "RST"
	flagset = StringToImapFlags(s)
	expected = imap.NewFlagSet(`\Seen`, `\Answered`, `\Deleted`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

	s = "DRST"
	flagset = StringToImapFlags(s)
	expected = imap.NewFlagSet(`\Seen`, `\Answered`, `\Deleted`, `\Draft`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

	s = "DFRST"
	flagset = StringToImapFlags(s)
	expected = imap.NewFlagSet(`\Seen`, `\Answered`, `\Deleted`, `\Draft`, `\Flagged`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

	s = "ABCDEFGHIDFRST"
	flagset = StringToImapFlags(s)
	expected = imap.NewFlagSet(`\Seen`, `\Answered`, `\Deleted`, `\Draft`, `\Flagged`)

	if !reflect.DeepEqual(expected, flagset) {
		t.Fatalf("Expecting flagset %v, found %v", expected, flagset)
	}

}
