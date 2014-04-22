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
	"sort"
	"strconv"
	"strings"

	"code.google.com/p/go-imap/go1/imap"
	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
)

type ImapConnectionConfig struct {
	Hostname string
	Username string
	Password string
}

type ImapFolder struct {
	folder      *Mailfolder
	store       *ImapStore
	metadatadir string
	uidvalidity uint32
	imappath    string
	expunge     bool
	messages    map[uint32]*ImapMessageInfo
	client      *imap.Client
	logger      *log.Logger
	e           *errors.Error
	dryrun      bool
}

type ImapMessageInfo struct {
	MessageInfo
}

var (
	ImapFlagsMap = [][]string{
		{`\Seen`, "S"},
		{`\Answered`, "R"},
		{`\Deleted`, "T"},
		{`\Draft`, "D"},
		{`\Flagged`, "F"},
	}
)

func (m *ImapFolder) getImapClient() (*imap.Client, error) {
	if m.client != nil && m.client.State() != imap.Closed {
		return m.client, nil
	}

	client, err := m.store.newImapClient()

	if err != nil {
		m.logger.Debug("Connection error:", err)
		return nil, err
	}

	m.client = client
	return client, nil
}

func ImapFlagsToString(flagset imap.FlagSet) string {
	var flags string

	for _, v := range ImapFlagsMap {
		if _, ok := flagset[v[0]]; ok {
			flags += v[1]
		}
	}
	rs := runeSlice(flags)
	sort.Sort(rs)

	return string(rs)
}

func StringToImapFlags(flags string) imap.FlagSet {
	flagset := imap.NewFlagSet()

	for _, v := range ImapFlagsMap {
		if strings.Contains(flags, v[1]) {
			flagset[v[0]] = true
		}
	}
	return flagset
}

func NewImapFolder(folder *Mailfolder, metadatadir string, store *ImapStore, uidvalidity uint32, dryrun bool) (m *ImapFolder, err error) {
	logprefix := fmt.Sprintf("store: %s, imapfolder: %s", store.Name(), folder)
	errprefix := fmt.Sprintf("store: %s, imapfolder: %s", store.Name(), folder)
	logger := log.GetLogger(logprefix, store.globalconfig.LogLevel)
	e := errors.New(errprefix)

	separator, err := store.Separator()
	if err != nil {
		return nil, e.E(err)
	}

	imappath := FolderToStorePath(folder, separator)
	m = &ImapFolder{
		folder:      folder,
		store:       store,
		metadatadir: metadatadir,
		uidvalidity: uidvalidity,
		imappath:    imappath,
		expunge:     store.config.Expunge,
		messages:    make(map[uint32]*ImapMessageInfo),
		client:      nil,
		logger:      logger,
		e:           e,
		dryrun:      dryrun,
	}

	client, err := m.getImapClient()
	if err != nil {
		return nil, m.e.E(err)
	}

	m.client = client

	_, err = client.Select(m.imappath, false)
	if err != nil {
		return nil, m.e.E(err)
	}

	return m, nil
}

func (m *ImapFolder) UpdateMessageList() error {
	m.messages = make(map[uint32]*ImapMessageInfo)

	if m.dryrun && !m.store.HasFolder(m.folder) {
		return nil
	}

	client, err := m.getImapClient()
	if err != nil {
		return m.e.E(err)
	}

	m.logger.Debug("Mailbox status:")
	for _, line := range strings.Split(client.Mailbox.String(), "\n") {
		m.logger.Debug(line)
	}

	set, err := imap.NewSeqSet("1:*")
	if err != nil {
		return m.e.E(err)
	}

	cmd, err := client.Send("UID FETCH", set, "(UID FLAGS)")
	if err != nil {
		return m.e.E(err)
	}

	// Process responses while the command is running
	m.logger.Debug("Most recent messages:")
	for cmd.InProgress() {
		// Wait for the next response (no timeout)
		err := client.Recv(-1)
		if err != nil {
			return m.e.E(err)
		}

		// Process command data
		for _, rsp := range cmd.Data {
			var uid uint32 = rsp.MessageInfo().Attrs["UID"].(uint32)
			flags := ImapFlagsToString(imap.AsFlagSet(rsp.MessageInfo().Attrs["FLAGS"]))

			//m.log.Debugf("uid: %d, flags: %s", uid, flags)
			m.messages[uid] = &ImapMessageInfo{MessageInfo{uid, flags, false}}
		}
		cmd.Data = nil

		// Process unilateral server data
		for _, rsp := range client.Data {
			m.logger.Debug("Server data: ", rsp)
		}
		client.Data = nil
	}

	// Check command completion status
	if rsp, err := cmd.Result(imap.OK); err != nil {
		if err == imap.ErrAborted {
			m.logger.Debug("Fetch command aborted")
		} else {
			m.logger.Debug("Fetch error: ", rsp.Info)
		}
	}

	return nil
}

func (m *ImapFolder) HasUID(uid uint32) bool {
	if _, ok := m.messages[uid]; ok {
		return true
	}
	return false
}

func (m *ImapFolder) IsIgnored(uid uint32) bool {
	if m, ok := m.messages[uid]; ok {
		if m.Ignore {
			return true
		}
	}
	return false
}

func (m *ImapFolder) GetFlags(uid uint32) (flags string, err error) {
	if message, ok := m.messages[uid]; ok {
		return message.Flags, nil
	}

	err = fmt.Errorf("Cannot find message with uid:", uid)
	return
}

func (m *ImapFolder) SetFlags(uid uint32, flags string) (err error) {
	client, err := m.getImapClient()
	if err != nil {
		return m.e.E(err)
	}

	m.logger.Debug("Mailbox status:")
	for _, line := range strings.Split(client.Mailbox.String(), "\n") {
		m.logger.Debug(line)
	}

	set, _ := imap.NewSeqSet(strconv.FormatUint(uint64(uid), 10))
	flagset := StringToImapFlags(flags)
	cmd, err := imap.Wait(client.UIDStore(set, "FLAGS", flagset.String()))

	// Check command completion status
	rsp, err := cmd.Result(imap.OK)
	if err != nil {
		if err == imap.ErrAborted {
			m.logger.Debug("UIDStore command aborted")
		} else {
			m.logger.Debug("UIDStore error:", rsp.Info)
		}
		return m.e.E(err)
	}

	m.messages[uid].Flags = flags
	return

}

func (m *ImapFolder) ReadMessage(uid uint32) ([]byte, error) {
	client, err := m.getImapClient()
	if err != nil {
		return nil, err
	}

	var body []byte

	set, err := imap.NewSeqSet(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return nil, m.e.E(err)
	}
	cmd, err := client.Send("UID FETCH", set, "(BODY[])")
	if err != nil {
		return nil, m.e.E(err)
	}

	for cmd.InProgress() {
		// Wait for the next response (no timeout)
		err := client.Recv(-1)
		if err != nil {
			return nil, m.e.E(err)
		}
		// Process command data
		for _, rsp := range cmd.Data {
			body = imap.AsBytes(rsp.MessageInfo().Attrs["BODY[]"])
			m.logger.Debug("UID: ", rsp.MessageInfo().UID)
		}
		cmd.Data = nil

		// Process unilateral server data
		for _, rsp := range client.Data {
			m.logger.Debug("Server data:", rsp)
		}
		client.Data = nil
	}

	return body, nil

}

func (m *ImapFolder) AddMessage(uid uint32, flags string, body []byte) (newuid uint32, err error) {
	client, err := m.getImapClient()
	if err != nil {
		return 0, m.e.E(err)
	}

	literal := imap.NewLiteral(body)
	flagset := StringToImapFlags(flags)

	cmd, err := imap.Wait(client.Append(m.imappath, flagset, nil, literal))

	// Check command completion status
	rsp, err := cmd.Result(imap.OK)
	if err != nil {
		if err == imap.ErrAborted {
			m.logger.Debug("Append command aborted")
		} else {
			m.logger.Debug("Append error: ", rsp.Info)
		}
		return 0, m.e.E(err)
	}

	if len(rsp.Fields) >= 3 {
		m.logger.Debug("rsp.Fields[0]", rsp.Fields[0])
		newuid = imap.AsNumber(rsp.Fields[2])
	} else {
		m.logger.Debug("Not enought response Fields")
		err = fmt.Errorf("Not enought response Fields")
		return 0, m.e.E(err)
	}

	messageinfo := ImapMessageInfo{MessageInfo{newuid, flags, false}}
	m.messages[newuid] = &messageinfo
	m.logger.Debugf("Registering message. uid: %d, messageinfo: %v", newuid, messageinfo)
	return
}

func (m *ImapFolder) DeleteMessage(uid uint32) error {

	if !m.HasUID(uid) {
		return m.e.E(fmt.Errorf("uid: %d, doesn't exists"))
	}

	// Set Deleted flags. Message will be expunged on folder close if m.expunge == true
	err := m.SetFlags(uid, "T")
	if err != nil {
		return m.e.E(err)
	}

	delete(m.messages, uid)
	return nil
}

func (m *ImapFolder) Update(srcuid uint32) (uint32, error) {
	return srcuid, nil
}

func (m *ImapFolder) GetMessages() map[uint32]*MessageInfo {
	messages := make(map[uint32]*MessageInfo, 0)

	for uid, message := range m.messages {
		messages[uid] = &message.MessageInfo
	}

	return messages
}

func (m *ImapFolder) GetIgnoredMessages() []uint32 {
	messages := make([]uint32, 0)

	for _, message := range m.messages {
		if message.Ignore {
			messages = append(messages, message.UID)
		}
	}
	return messages

}

func (m *ImapFolder) Close() (err error) {
	if m.client == nil {
		return
	}

	m.client.Close(m.expunge)
	m.client.Logout(10)

	return
}
