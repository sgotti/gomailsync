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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sgotti/gomailsync/errors"
	"github.com/sgotti/gomailsync/log"
)

type MaildirFolder struct {
	folder        *Mailfolder
	store         *MaildirStore
	maildir       string
	metadatadir   string
	folderUID     string
	messages      map[uint32]*MaildirMessageInfo
	nextTempUID   uint32
	lastTime      int64
	lastTimeSeq   uint32
	logger        *log.Logger
	infoSeparator rune
	e             *errors.Error
	dryrun        bool
}

type MaildirMessageInfo struct {
	MessageInfo

	// Filename without separator + flags
	Filename  string
	Subdir    string // cur or new
	Temporary bool
}

func (m *MaildirFolder) getTimeSeq() (int64, uint32) {
	curtime := time.Now().Unix()

	if curtime == m.lastTime {
		m.lastTimeSeq++
	} else {
		m.lastTime = curtime
		m.lastTimeSeq = 0
	}

	return curtime, m.lastTimeSeq
}

func (m *MaildirFolder) getNextTempUID() uint32 {
	defer func() { m.nextTempUID -= 1 }()
	return m.nextTempUID
}

func (m *MaildirFolder) getNextFreeUID() (uint32, error) {
	for i := uint32(0); i < math.MaxUint32; i++ {
		if _, ok := m.messages[i]; !ok {
			return i, nil
		}
	}

	err := fmt.Errorf("Cannot find a free uid")
	return 0, err
}

func (m *MaildirFolder) generateFilename(uid uint32) (string, error) {
	time, timeseq := m.getTimeSeq()
	hostname, err := os.Hostname()
	if err != nil {
		m.logger.Debug("Error getting hostname")
		return "", err
	}
	filename := fmt.Sprintf("%d_%d.%d.%s,u=%d,f=%s", time, timeseq, os.Getpid(), hostname, uid, m.folderUID)
	return filename, nil
}

func (m *MaildirFolder) generateFullFilename(uid uint32, flags string) (string, error) {
	filename, err := m.generateFilename(uid)
	if err != nil {
		return "", err
	}

	fullfilename := filename + string(m.infoSeparator) + "2," + flags
	return fullfilename, nil
}

// Return filename and ordered flags
func (m *MaildirFolder) splitFilename(fullfilename string) (string, string, error) {
	split := strings.FieldsFunc(fullfilename, func(r rune) bool {
		return r == m.infoSeparator
	})
	if len(split) != 2 {
		err := fmt.Errorf("Wrong filename format:", fullfilename)
		return "", "", err
	}

	if !strings.HasPrefix(split[1], "2,") {
		err := fmt.Errorf("Wrong filename format:", fullfilename)
		return "", "", err
	}

	flags := strings.Replace(split[1], "2,", "", 1)
	outflags := CleanFlags(flags)
	return split[0], outflags, nil
}

func (m *MaildirFolder) registerMessage(UID uint32, flags string, filename string, subdir string, temporary bool) {
	messageinfo := MaildirMessageInfo{MessageInfo{UID, flags, false}, filename, subdir, temporary}
	m.messages[UID] = &messageinfo
	m.logger.Debugf("Registering message. uid: %d, messageinfo: %v", UID, messageinfo)
}

func (m *MaildirFolder) findFilepath(messageinfo *MaildirMessageInfo) (messagepath string, err error) {
	var ok bool = false
	var filename string
	var dupfilenames []string
	for _, d := range []string{"cur", "new"} {
		f, err := os.Open(filepath.Join(m.maildir, d))
		if err != nil {
			return "", err
		}
		defer f.Close()
		filenames, err := f.Readdirnames(0)
		if err != nil {
			return "", err
		}

		for _, n := range filenames {
			filename, _, err := m.splitFilename(n)
			if err != nil && d == "new" {
				if messageinfo.Filename == n {
					messagepath = filepath.Join(m.maildir, d, n)
				}
			}
			if err == nil && messageinfo.Filename == filename {
				if ok == true {
					dupfilenames = append(dupfilenames, n)
				} else {
					messagepath = filepath.Join(m.maildir, d, n)
					ok = true
				}
			}
		}
	}
	if len(dupfilenames) > 0 {
		err = fmt.Errorf("Duplicate files with same filename (%s): %v", filename, dupfilenames)
		return
	}

	return
}

func NewMaildirFolder(folder *Mailfolder, maildir string, metadatadir string, store *MaildirStore, folderUID string, dryrun bool) (m *MaildirFolder, err error) {
	logprefix := fmt.Sprintf("store: %s, maildirfolder: %s", store.Name(), folder)
	errprefix := logprefix
	logger := log.GetLogger(logprefix, store.globalconfig.LogLevel)
	e := errors.New(errprefix)

	switch store.config.UIDMapping {
	case "files":
	case "db":
		err := fmt.Errorf("UIDMapping of type \"db\" not yet implemented")
		return nil, e.E(err)

	default:
		err := fmt.Errorf("Wrong UIDMapping: \"%s\"", store.config.UIDMapping)
		return nil, e.E(err)
	}

	m = &MaildirFolder{
		folder:        folder,
		maildir:       maildir,
		metadatadir:   metadatadir,
		store:         store,
		messages:      make(map[uint32]*MaildirMessageInfo),
		nextTempUID:   math.MaxUint32,
		lastTime:      0,
		lastTimeSeq:   0,
		logger:        logger,
		infoSeparator: ':', // TODO At the moment I don't care for filesystems not accepting colon in filenames
		e:             e,
		dryrun:        dryrun,
	}

	m.folderUID = folderUID

	return
}

func (m *MaildirFolder) UpdateMessageList() error {
	m.messages = make(map[uint32]*MaildirMessageInfo)

	if m.dryrun && !m.store.HasFolder(m.folder) {
		return nil
	}

	re := regexp.MustCompile(`,u=(\d+),f=([A-Za-z0-9]+)`)

	for _, d := range []string{"cur", "new"} {
		f, err := os.Open(filepath.Join(m.maildir, d))
		if err != nil {
			return m.e.E(err)
		}
		defer f.Close()
		filenames, err := f.Readdirnames(0)
		if err != nil {
			m.logger.Error("Error: ", err)
			return m.e.E(err)
		}

		for _, n := range filenames {
			filename, flags, err := m.splitFilename(n)

			if err != nil {
				if d != "new" {
					m.logger.Debugf("Split error: %s. Ignoring message filename: %s/%s", err, d, n)
					continue
				} else {
					// Accept a file without flags if it doesn't contains InfoSeparator
					if strings.Contains(n, string(m.infoSeparator)) {
						m.logger.Debugf("Not accepting message filename %s in \"new\" folder without flags but with separator %s", n, m.infoSeparator)
						continue
					} else {
						filename = n
					}
				}
			}

			match := re.FindStringSubmatch(filename)

			if len(match) < 2 {
				m.logger.Debugf("Assuming as new message: %s", filename)
				m.registerMessage(m.getNextTempUID(), flags, filename, d, true)
			} else {
				if m.folderUID == match[2] {
					uid, _ := strconv.ParseUint(match[1], 10, 32)
					if m.HasUID(uint32(uid)) {
						m.logger.Warningf("Message with filename \"%s\" containing uid %d already existent! Setting this uid to be ignored by sync alghoritm.", filename, uid)
						// for security remove previous messageinfo filename, subdir, etc...
						m.messages[uint32(uid)].Filename = ""
						m.messages[uint32(uid)].Subdir = ""
						m.messages[uint32(uid)].Ignore = true
						continue
					}
					m.registerMessage(uint32(uid), flags, filename, d, false)
				} else {
					m.logger.Debugf("Message folderuid: %s different from folderuid: %s. Assuming %s as new message", match[2], m.folderUID, filename)
					m.registerMessage(m.getNextTempUID(), flags, filename, d, true)
				}
			}
		}
	}

	return nil
}

func (m *MaildirFolder) HasUID(uid uint32) bool {
	if _, ok := m.messages[uid]; ok {
		return true
	}
	return false
}

func (m *MaildirFolder) IsIgnored(uid uint32) bool {
	if m, ok := m.messages[uid]; ok {
		if m.Ignore {
			return true
		}
	}
	return false
}

func (m *MaildirFolder) GetFlags(uid uint32) (flags string, err error) {
	if message, ok := m.messages[uid]; ok {
		return message.Flags, nil
	}

	err = fmt.Errorf("Cannot find message with uid: %d", uid)
	return "", m.e.E(err)
}

func (m *MaildirFolder) SetFlags(uid uint32, flags string) (err error) {
	message, ok := m.messages[uid]
	if !ok {
		err = fmt.Errorf("Cannot find message with uid: %d", uid)
		return m.e.E(err)
	}

	srcfilename := message.Filename
	srcfilepath, err := m.findFilepath(message)
	if err != nil {
		return m.e.E(err)
	}

	dstfullfilename := srcfilename + string(m.infoSeparator) + "2," + flags
	dstfilepath := filepath.Join(m.maildir, message.Subdir, dstfullfilename)

	err = os.Rename(srcfilepath, dstfilepath)
	if err != nil {
		return m.e.E(err)
	}

	message.Flags = flags
	return
}

func (m *MaildirFolder) ReadMessage(uid uint32) ([]byte, error) {
	messageinfo := m.messages[uid]
	m.logger.Debug("maildirmessageinfo:", messageinfo)

	filepath, err := m.findFilepath(messageinfo)
	if err != nil {
		return nil, m.e.E(err)
	}
	if filepath == "" {
		err := fmt.Errorf("Cannot find file for message uid: %d on filesystem.", uid)
		return nil, m.e.E(err)
	}

	m.logger.Debug("filepath:", filepath)
	fi, err := os.Open(filepath)
	if err != nil {
		m.logger.Debug("Cannot open file:", filepath)
		return nil, m.e.E(err)
	}

	buf, err := ioutil.ReadAll(fi)

	return buf, m.e.E(err)
}

func (m *MaildirFolder) AddMessage(srcuid uint32, flags string, body []byte) (uint32, error) {

	uid, err := m.getNextFreeUID()
	if err != nil {
		return 0, m.e.E(err)
	}

	filename, err := m.generateFilename(uid)
	if err != nil {
		return 0, m.e.E(err)
	}

	fullfilename := filename + string(m.infoSeparator) + "2," + flags

	m.logger.Debug("filename:", filename)
	tmpfilepath := filepath.Join(m.maildir, "tmp", fullfilename)
	filepath := filepath.Join(m.maildir, "cur", fullfilename)

	fo, err := os.Create(tmpfilepath)
	if err != nil {
		return 0, m.e.E(err)
	}

	defer func() {
		if err := fo.Close(); err != nil {
			m.logger.Error("file close error")
		}
	}()

	w := bufio.NewWriter(fo)

	if _, err := w.Write(body); err != nil {
		return 0, m.e.E(err)
	}
	if err = w.Flush(); err != nil {
		return 0, m.e.E(err)
	}
	if err = os.Rename(tmpfilepath, filepath); err != nil {
		return 0, m.e.E(err)
	}

	m.registerMessage(uid, flags, filename, "cur", false)

	return uint32(uid), nil
}

func (m *MaildirFolder) DeleteMessage(uid uint32) (err error) {
	message, ok := m.messages[uid]
	if !ok {
		err = fmt.Errorf("Cannot find message with uid: %d", uid)
		return m.e.E(err)
	}

	filepath, err := m.findFilepath(message)
	if err != nil {
		return err
	}
	if filepath != "" {
		rmerr := os.Remove(filepath)
		// Ignore if file does not exists
		if rmerr != nil {
			m.logger.Debugf("remove failed: %s. Ignoring ", rmerr)
		}
	}
	delete(m.messages, uid)

	return
}

func (m *MaildirFolder) Update(srcuid uint32) (outsrcuid uint32, err error) {
	outsrcuid = srcuid
	message, ok := m.messages[srcuid]
	if !ok {
		err = fmt.Errorf("Cannot find message with uid: %d", srcuid)
		return 0, m.e.E(err)
	}

	srcfilepath, err := m.findFilepath(message)

	// Generate a new uid. Do not use temporary uid.
	if message.Temporary {
		outsrcuid, err = m.getNextFreeUID()
		if err != nil {
			return 0, m.e.E(err)
		}
	}

	dstfilename, err := m.generateFilename(outsrcuid)
	if err != nil {
		return 0, m.e.E(err)
	}

	dstfullfilename := dstfilename + string(m.infoSeparator) + "2," + message.Flags
	// TODO A file with flags cannot live in folder "new" (also if mutt does this). Add an option to choose the desired behavior...
	dstfilepath := filepath.Join(m.maildir, "cur", dstfullfilename)

	//m.logger.Debugf("srcfilepath: %s, dstfilepath: %s", srcfilepath, dstfilepath)
	err = os.Rename(srcfilepath, dstfilepath)
	if err != nil {
		return 0, m.e.E(err)
	}

	message.UID = outsrcuid
	message.Subdir = "cur"
	message.Filename = dstfilename
	message.Temporary = false
	return
}

func (m *MaildirFolder) GetMessages() map[uint32]*MessageInfo {
	messages := make(map[uint32]*MessageInfo, 0)

	for _, message := range m.messages {
		messages[message.UID] = &message.MessageInfo
	}
	return messages

}

func (m *MaildirFolder) GetIgnoredMessages() []uint32 {
	messages := make([]uint32, 0)

	for _, message := range m.messages {
		if message.Ignore {
			messages = append(messages, message.UID)
		}
	}
	return messages
}

func (m *MaildirFolder) Close() (err error) {
	return
}
