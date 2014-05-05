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
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
)

type MaildirStore struct {
	globalconfig *config.Config
	config       *config.StoreConfig
	name         string
	maildir      string
	metadatadir  string
	separator    rune
	folders      []*Mailfolder
	logger       *log.Logger
	e            *errors.Error
	dryrun       bool
}

func (m *MaildirStore) isInbox(relpath string) bool {
	if filepath.Clean(relpath) == filepath.Clean(m.config.InboxPath) {
		return true
	}
	return false
}

func (m *MaildirStore) maildirPath(name foldername) string {
	folderpath := FolderToStorePath(name, m.separator)
	if StrsEquals(name, []string{"INBOX"}) {
		folderpath = filepath.Clean(m.config.InboxPath)
	}
	return folderpath
}

func (m *MaildirStore) readFolderUID(path string) (folderUID string, err error) {
	f, err := os.Open(path)
	if err != nil {
		m.logger.Debugf("open failed")
		return "", nil
	}
	scanner := bufio.NewScanner(f)
	scanner.Scan()
	folderUID = scanner.Text()

	if len(folderUID) != 32 {
		err := fmt.Errorf("Wrong folderUID: \"%s\". Something strange happened", folderUID)
		return "", err
	}

	return
}

func (m *MaildirStore) getFolderUID(name foldername) (folderUID string, err error) {
	foldermaildir := filepath.Join(m.maildir, m.maildirPath(name))
	foldermetadatadir := filepath.Join(m.metadatadir, FolderToStorePath(name, os.PathSeparator))

	mddirfilepath := filepath.Join(foldermetadatadir, "folderuid")
	maildirfilepath := filepath.Join(foldermaildir, ".gomailsync-folderuid")

	folderUID1, err := m.readFolderUID(mddirfilepath)
	if err != nil {
		return "", m.e.E(err)
	}
	folderUID2, err := m.readFolderUID(maildirfilepath)
	if err != nil {
		return "", m.e.E(err)
	}

	if folderUID1 != folderUID2 {
		return "", fmt.Errorf("FolderUID in metadatadir: \"%s\" and in maildir: \"%s\" are different!!!", folderUID1, folderUID2)
	}

	folderUID = folderUID1

	return
}

func (m *MaildirStore) writeFolderUID(path string, folderUID string) (err error) {
	fo, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		if err := fo.Close(); err != nil {
			m.logger.Error("file close error")
		}
	}()

	w := bufio.NewWriter(fo)

	if _, err := w.WriteString(folderUID); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = fo.Sync(); err != nil {
		return err
	}

	return nil
}

func generateFolderUID() (folderUID string, err error) {
	c := 10
	b := make([]byte, c)
	_, err = rand.Read(b)
	if err != nil {
		return "", err
	}
	h := md5.New()
	h.Write(b)

	folderUID = fmt.Sprintf("%x", h.Sum(nil))

	return
}

func NewMaildirStore(globalconfig *config.Config, config *config.StoreConfig, basemetadatadir string, dryrun bool) (m *MaildirStore, err error) {
	name := config.Name
	logprefix := fmt.Sprintf("maildistore: %s", name)
	errprefix := fmt.Sprintf("maildistore: %s", name)
	logger := log.GetLogger(logprefix, globalconfig.LogLevel)
	e := errors.New(errprefix)

	metadatadir := filepath.Join(basemetadatadir, name)
	maildir := config.Maildir

	err = os.MkdirAll(metadatadir, 0777)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(maildir, 0777)
	if err != nil {
		m.logger.Error("Error:", err)
		return
	}

	m = &MaildirStore{
		globalconfig: globalconfig,
		config:       config,
		name:         name,
		maildir:      maildir,
		metadatadir:  metadatadir,
		separator:    config.Separator,
		folders:      make([]*Mailfolder, 0),
		logger:       logger,
		e:            e,
		dryrun:       dryrun,
	}

	err = m.UpdateFolderList()
	return
}

func (m *MaildirStore) CreateFolder(name foldername) (err error) {
	foldermaildir := filepath.Join(m.maildir, m.maildirPath(name))

	for _, d := range []string{"cur", "new", "tmp"} {
		err := os.MkdirAll(filepath.Join(foldermaildir, d), 0777)
		if err != nil {
			m.logger.Error("Error:", err)
		}
	}

	foldermetadatadir := filepath.Join(m.metadatadir, FolderToStorePath(name, os.PathSeparator))

	mddirfilepath := filepath.Join(foldermetadatadir, "folderuid")
	maildirfilepath := filepath.Join(foldermaildir, ".gomailsync-folderuid")

	err = os.MkdirAll(foldermetadatadir, 0777)
	if err != nil {
		return m.e.E(err)
	}

	folderUID, err := m.getFolderUID(name)
	if err != nil {
		return m.e.E(err)
	}

	// Create folderUID files
	if folderUID == "" {
		folderUID, err = generateFolderUID()
		if err != nil {
			return m.e.E(err)
		}

		err = m.writeFolderUID(mddirfilepath, folderUID)
		if err != nil {
			return m.e.E(err)
		}
		m.writeFolderUID(maildirfilepath, folderUID)
		if err != nil {
			// Remove previous file
			os.Remove(mddirfilepath)
			return m.e.E(err)
		}
	}

	m.logger.Debugf("folderUID: %s", folderUID)

	// Add folder to the list
	m.folders = append(m.folders, &Mailfolder{Name: name, Excluded: false})
	return err
}

func (m *MaildirStore) SetFolderExcluded(name foldername, excluded bool) error {
	if f := m.getFolder(name); f != nil {
		f.Excluded = excluded
		return nil
	}
	return m.e.E(fmt.Errorf("Folder %s, doesn't exists", name))
}

func (m *MaildirStore) getFolder(name foldername) *Mailfolder {
	for _, f := range m.folders {
		if StrsEquals(name, f.Name) {
			return f
		}
	}
	return nil
}

func (m *MaildirStore) HasFolder(name foldername) bool {
	if f := m.getFolder(name); f != nil {
		return true
	}
	return false
}

func (m *MaildirStore) UpdateFolderList() error {
	m.folders = make([]*Mailfolder, 0)
	subdirs := []string{"cur", "new", "tmp"}
	err := filepath.Walk(m.maildir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && !StringInSlice(filepath.Base(path), subdirs) {
			var ok uint8 = 0
			f, err := os.Open(path)
			if err != nil {
				return m.e.E(err)
			}
			defer f.Close()
			filenames, err := f.Readdirnames(0)
			if err != nil {
				return m.e.E(err)
			}

			for _, n := range filenames {
				if StringInSlice(n, subdirs) {
					ok++
				}
			}

			if ok == 3 {
				relpath, err := filepath.Rel(m.maildir, path)
				if err != nil {
					return m.e.E(err)
				}

				// Verify that the if relpath is inbox (case insensitive) then its the configured inbox
				if strings.ToLower(filepath.Clean(relpath)) == "inbox" && !m.isInbox(relpath) {
					err := fmt.Errorf("directory with name \"%s\", doesn't match configured inbox path \"%s\"", filepath.Clean(relpath), (m.config.InboxPath))
					return m.e.E(err)
				}
				name := strings.Split(relpath, string(m.separator))
				// Is this path of the configured INBOX?
				if m.isInbox(relpath) {
					name = []string{"INBOX"}
				}
				folder := &Mailfolder{
					Name:     name,
					Excluded: false,
				}
				m.folders = append(m.folders, folder)
				m.logger.Debug("maildir folder:", folder)
			}
		}
		return nil
	})
	if err != nil {
		return m.e.E(err)
	}

	err = applyRegExpPatterns(m, m.folders)
	if err != nil {
		return m.e.E(err)
	}

	return nil
}

func (m *MaildirStore) Separator() (rune, error) {
	return m.separator, nil
}

func (m *MaildirStore) GetFolders() []Mailfolder {
	folders := make([]Mailfolder, len(m.folders))
	for i, f := range m.folders {
		folders[i] = *f
	}
	return folders
}

func (m *MaildirStore) GetMailfolderManager(name foldername) (manager MailfolderManager, err error) {
	maildir := filepath.Join(m.maildir, m.maildirPath(name))

	if !m.HasFolder(name) && !m.dryrun {
		err = m.CreateFolder(name)
		if err != nil {
			return nil, m.e.E(err)
		}
	}

	folderUID, err := m.getFolderUID(name)
	if err != nil {
		return nil, m.e.E(err)
	}

	folder := m.getFolder(name)
	if folder == nil {
		return nil, m.e.E(fmt.Errorf("Cannot get folder %s", name))
	}
	manager, err = NewMaildirFolder(folder, maildir, m.metadatadir, m, folderUID, m.dryrun)

	return
}

func (m *MaildirStore) Name() string {
	return m.name
}

func (m *MaildirStore) Config() *config.StoreConfig {
	return m.config
}
