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
	"github.com/sgotti/gomailsync/config"
)

type StoreManager interface {
	CreateFolder(foldername) error
	UpdateFolderList() error
	Separator() (rune, error)
	SetFolderExcluded(name foldername, excluded bool) error
	HasFolder(foldername) bool
	GetFolders() []Mailfolder
	GetMailfolderManager(foldername) (MailfolderManager, error)

	Name() string
	Config() *config.StoreConfig
}
