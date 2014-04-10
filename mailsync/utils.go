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
	"os"
	"sort"
	"strings"
)

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func FolderToStorePath(folder *Mailfolder, separator rune) string {

	// TODO Escape possible(?) os path separator in folder name
	name := strings.Join(folder.Name, string(separator))

	return name
}

func MkdirIfNotExists(name string) (err error) {
	if _, err = os.Stat(name); os.IsNotExist(err) {
		err = os.Mkdir(name, 0777)
	}
	return
}

type runeSlice []rune

func (s runeSlice) Len() int           { return len(s) }
func (s runeSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s runeSlice) Less(i, j int) bool { return s[i] < s[j] }

func CleanFlags(flags string) string {
	runeflags := runeSlice(flags)
	flagsmap := make(map[rune]bool)
	for _, flag := range runeflags {
		flagsmap[flag] = true
	}

	var outflags runeSlice
	for flags, _ := range flagsmap {
		outflags = append(outflags, flags)
	}

	sort.Sort(outflags)
	return string(outflags)
}

func addFlags(flags string, newflags string) string {
	return CleanFlags(flags + "newflags")
}

func removeFlags(flags string, newflags string) string {
	runenewflags := runeSlice(newflags)
	outflags := flags
	for _, newflag := range runenewflags {
		outflags = strings.Replace(outflags, string(newflag), "", -1)
	}
	return CleanFlags(outflags)
}
