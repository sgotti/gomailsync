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

type foldername []string

type Mailfolder struct {
	Name     foldername
	Excluded bool
}

func (f Mailfolder) String() string {
	return FolderToStorePath(f.Name, '/')
}

func (f *Mailfolder) Equals(f2 *Mailfolder) bool {

	if len(f.Name) != len(f2.Name) {
		return false
	}

	for i := 0; i < len(f.Name); i++ {
		if f.Name[i] != f2.Name[i] {
			return false
		}
	}

	return true
}

type MailfolderManager interface {
	UpdateMessageList() error

	HasUID(uint32) bool
	IsIgnored(uint32) bool

	GetFlags(uint32) (string, error)
	SetFlags(uint32, string) error

	ReadMessage(uint32) ([]byte, error)

	AddMessage(uint32, string, []byte) (uint32, error)
	DeleteMessage(uint32) error
	Update(uint32) (uint32, error)

	GetMessages() map[uint32]*MessageInfo
	GetIgnoredMessages() []uint32

	Close() error
}
