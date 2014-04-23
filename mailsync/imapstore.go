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
	"bufio"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mxk/go-imap/imap"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
)

type ImapStore struct {
	globalconfig *config.Config
	config       *config.StoreConfig
	name         string
	metadatadir  string
	client       *imap.Client
	separator    rune
	folders      []*Mailfolder
	logger       *log.Logger
	e            *errors.Error
	dryrun       bool
	sync.Mutex
}

func (m *ImapStore) newImapClient() (client *imap.Client, err error) {
	if m.config.Tls && m.config.Starttls {
		return nil, fmt.Errorf("Both tls and starttls enabled. Only one of them is permitted.")
	}

	addr := m.config.Host
	if m.config.Port != 0 {
		addr = addr + ":" + strconv.FormatUint(uint64(m.config.Port), 10)
	}
	var tlsconfig *tls.Config
	if !m.config.Validateservercert {
		tlsconfig = &tls.Config{InsecureSkipVerify: true}
	}
	if m.config.Tls {
		client, err = imap.DialTLS(addr, tlsconfig)
		if err != nil {
			return nil, err
		}
	} else {
		client, err = imap.Dial(addr)
		if err != nil {
			return nil, err
		}
	}

	if m.globalconfig.LogLevel == "debug" && m.globalconfig.DebugImap {
		client.SetLogMask(imap.LogAll)
	}

	// Print server greeting (first response in the unilateral server data queue)
	m.logger.Debug("Server says hello: ", client.Data[0].Info)
	client.Data = nil

	if m.config.Starttls && !client.Caps["STARTTLS"] {
		return nil, fmt.Errorf("Server doesn't support STARTTLS")
	}

	if m.config.Starttls && client.Caps["STARTTLS"] {
		_, err = client.StartTLS(tlsconfig)
		if err != nil {
			return nil, err
		}
	}

	// Authenticate
	if client.State() == imap.Login {
		_, err = client.Login(m.config.Username, m.config.Password)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

func (m *ImapStore) getImapClient() (*imap.Client, error) {
	if m.client != nil && m.client.State() != imap.Closed {
		return m.client, nil
	}

	client, err := m.newImapClient()
	if err != nil {
		return nil, err
	}

	m.client = client

	return client, nil
}

func (m *ImapStore) getUIDValidity(folder *Mailfolder) (uidvalidity uint32, err error) {
	// Get UIDValidity from the server
	imappath := FolderToStorePath(folder, m.separator)
	client, err := m.getImapClient()
	if err != nil {
		return 0, m.e.E(err)
	}

	_, err = client.Select(imappath, true)
	if err != nil {
		return 0, m.e.E(err)
	}
	defer client.Close(false)

	m.logger.Debug("Mailbox status:")
	for _, line := range strings.Split(client.Mailbox.String(), "\n") {
		m.logger.Debug(line)
	}
	serveruidvalidity := client.Mailbox.UIDValidity

	// Verify that metadatadir has already an uidvalidity or create it from the server provided one
	var mduidvalidity uint32
	foldermetadatadir := filepath.Join(m.metadatadir, FolderToStorePath(folder, os.PathSeparator))
	uidvaliditypath := filepath.Join(foldermetadatadir, "uidvalidity")
	f, err := os.Open(uidvaliditypath)
	if err != nil {
		fo, err := os.Create(uidvaliditypath)
		if err != nil {
			return 0, m.e.E(err)
		}

		defer func() {
			if err := fo.Close(); err != nil {
				m.logger.Errorf("file close error")
			}
		}()

		w := bufio.NewWriter(fo)

		if _, err := w.WriteString(strconv.FormatUint(uint64(serveruidvalidity), 10)); err != nil {
			return 0, m.e.E(err)
		}
		if err = w.Flush(); err != nil {
			return 0, m.e.E(err)
		}
		if err = fo.Sync(); err != nil {
			return 0, m.e.E(err)
		}

		mduidvalidity = serveruidvalidity
	} else {
		defer f.Close()
		r := bufio.NewReader(f)
		scanner := bufio.NewScanner(r)
		scanner.Scan()
		uidvaliditystr := scanner.Text()

		if len(uidvaliditystr) == 0 {
			err := fmt.Errorf("Wrong uidvalidity %s. Something strange happened", uidvalidity)
			return 0, m.e.E(err)
		}

		u, err := strconv.ParseUint(uidvaliditystr, 10, 32)
		if err != nil {
			return 0, m.e.E(err)
		}
		mduidvalidity = uint32(u)
	}
	m.logger.Debugf("serveruidvalidity: %d", serveruidvalidity)
	m.logger.Debugf("mduidvalidity: %d", mduidvalidity)

	if serveruidvalidity != mduidvalidity {
		err = fmt.Errorf("IMAP server uidvalidity %d doesn't match saved uidvalidity %d", serveruidvalidity, mduidvalidity)
		return 0, m.e.E(err)
	}

	return serveruidvalidity, nil
}

func NewImapStore(globalconfig *config.Config, config *config.StoreConfig, basemetadatadir string, dryrun bool) (m *ImapStore, err error) {
	name := config.Name
	logprefix := fmt.Sprintf("imapstore: %s", name)
	errprefix := fmt.Sprintf("imapstore: %s", name)
	logger := log.GetLogger(logprefix, globalconfig.LogLevel)
	e := errors.New(errprefix)

	metadatadir := filepath.Join(basemetadatadir, name)

	err = os.MkdirAll(metadatadir, 0777)
	if err != nil {
		return nil, err
	}

	m = &ImapStore{
		globalconfig: globalconfig,
		config:       config,
		name:         name,
		metadatadir:  metadatadir,
		client:       nil,
		separator:    0,
		folders:      make([]*Mailfolder, 0),
		logger:       logger,
		e:            e,
	}

	_, err = m.getImapClient()
	if err != nil {
		return nil, m.e.E(err)
	}

	// Verify if the server supports uidplus extensions
	if !m.client.Caps["UIDPLUS"] {
		err = m.e.E(fmt.Errorf("Server doesn't provide UIDPLUS capability"))
		return
	}

	err = m.UpdateFolderList()
	return
}

func (m *ImapStore) CreateFolder(folder *Mailfolder) (err error) {
	client, err := m.getImapClient()
	if err != nil {
		return m.e.E(err)
	}

	_, err = imap.Wait(client.Create(FolderToStorePath(folder, m.separator)))
	if err != nil {
		return m.e.E(err)
	}

	return
}

func (m *ImapStore) HasFolder(folder *Mailfolder) bool {
	for _, f := range m.folders {
		if FolderToStorePath(folder, m.separator) == FolderToStorePath(f, m.separator) {
			return true
		}
	}
	return false
}

func (m *ImapStore) UpdateFolderList() error {
	m.folders = make([]*Mailfolder, 0)

	client, err := m.getImapClient()
	if err != nil {
		return m.e.E(err)
	}

	var cmd *imap.Command
	var rsp *imap.Response

	var separator rune

	cmd, err = imap.Wait(client.List("", "*"))
	if err != nil {
		return m.e.E(err)
	}

	// Print mailbox information
	m.logger.Debug("Folders:")
	for _, rsp = range cmd.Data {
		name := strings.Split(rsp.MailboxInfo().Name, string(rsp.MailboxInfo().Delim))
		if separator == 0 {
			separator, _ = utf8.DecodeRuneInString(rsp.MailboxInfo().Delim)
		}
		// Ignore \Noselect folders
		if _, ok := rsp.MailboxInfo().Attrs[`\Noselect`]; ok {
			continue
		}
		folder := &Mailfolder{
			Name:     name,
			Excluded: false,
		}
		m.folders = append(m.folders, folder)
		m.logger.Debugf("%v", rsp.MailboxInfo())
	}

	m.separator = separator

	return nil

}

func (m *ImapStore) Separator() (rune, error) {
	return m.separator, nil
}

func (m *ImapStore) GetFolders() []*Mailfolder {
	return m.folders
}

func (m *ImapStore) GetMailfolderManager(folder *Mailfolder) (manager MailfolderManager, err error) {
	m.Lock()
	defer m.Unlock()

	if !m.HasFolder(folder) {
		err = m.CreateFolder(folder)
		if err != nil {
			return nil, err
		}
	}

	foldermetadatadir := filepath.Join(m.metadatadir, FolderToStorePath(folder, os.PathSeparator))
	err = os.MkdirAll(foldermetadatadir, 0777)
	if err != nil {
		return nil, m.e.E(err)
	}

	uidvalidity, err := m.getUIDValidity(folder)
	if err != nil {
		return nil, m.e.E(err)
	}

	manager, err = NewImapFolder(folder, foldermetadatadir, m, uidvalidity, m.dryrun)

	if err != nil {
		return nil, m.e.E(err)
	}
	return
}

func (m *ImapStore) Name() string {
	return m.name
}

func (m *ImapStore) Config() *config.StoreConfig {
	return m.config
}
