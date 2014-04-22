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

package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/sgotti/gomailsync/log"
)

type Config struct {
	Syncgroups  []*SyncgroupConfig `toml:"syncgroup"`
	Stores      []*StoreConfig     `toml:"store"`
	Metadatadir string
	LogLevel    string
	DebugImap   bool
}

type SyncgroupConfig struct {
	Name            string
	Stores          []string
	Concurrentsyncs uint8
	SyncInterval    duration
	Deletemode      string
}

type StoreConfig struct {
	Name      string
	StoreType string

	// Folders Patterns matching.
	// The format is:
	// /pattern/
	// !/pattern/
	RegexpPatterns []string

	// Imap specific config options
	Host               string
	Port               uint16
	Username           string
	Password           string
	Starttls           bool
	Tls                bool
	Validateservercert bool
	Expunge            bool

	// Maildir specific config options
	Maildir string

	// INBOX Path
	InboxPath string

	// "files" for uid mapping inside file names, "db" for uid mapping in a db file
	UIDMapping string

	Separator rune
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func ParseConfig(conffilepath string) (conf *Config, err error) {
	logger := log.GetLogger(fmt.Sprintf("config"), "debug")
	logger.Debugf("ParseConfig")

	defaultStoreConfig := StoreConfig{Validateservercert: true, UIDMapping: "files", Separator: os.PathSeparator, InboxPath: "./INBOX"}

	var syncinterval duration
	syncinterval.Duration, _ = time.ParseDuration("10m")
	defaultSyncgroupConfig := SyncgroupConfig{Concurrentsyncs: 1, SyncInterval: syncinterval, Deletemode: "expunge"}

	var configfile map[string]interface{}
	_, err = toml.DecodeFile(conffilepath, &configfile)
	if err != nil {
		return nil, err
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	defMetadatadir := filepath.Join(u.HomeDir, ".gomailsync")

	conf = &Config{
		Metadatadir: defMetadatadir,
		LogLevel:    "info",
		DebugImap:   false,
	}

	// Hack. Prefill Syncgroups and Stores slices in conf struct with default values
	for k, v := range configfile {
		switch k {
		case "store":
			for i := 0; i < len(v.([]map[string]interface{})); i++ {
				storeconfig := defaultStoreConfig
				conf.Stores = append(conf.Stores, &storeconfig)
			}
		case "syncgroup":
			for i := 0; i < len(v.([]map[string]interface{})); i++ {
				syncgroupconfig := defaultSyncgroupConfig
				conf.Syncgroups = append(conf.Syncgroups, &syncgroupconfig)
			}
		}
	}
	err = toml.PrimitiveDecode(configfile, &conf)
	return
}

func VerifyConfig(config *Config) (err error) {
	validloglevels := []string{"error", "info", "debug"}
	if !StringInSlice(config.LogLevel, validloglevels) {
		return fmt.Errorf("Wrong store type: \"%s\". Valid types are: %s", config.LogLevel, validloglevels)
	}

	for _, storeconf := range config.Stores {
		if err = VerifyStoreConfig(config, storeconf); err != nil {
			return err
		}
	}
	for _, syncgroupconf := range config.Syncgroups {
		if err = VerifySyncGroupConfig(config, syncgroupconf); err != nil {
			return err
		}
	}
	return nil
}

func VerifyStoreConfig(globalconfig *Config, config *StoreConfig) (err error) {
	logger := log.GetLogger(fmt.Sprintf("%s", "config"), globalconfig.LogLevel)
	logger.Debugf("VerifyStoreConfig")

	if config.Name == "" {
		return fmt.Errorf("Store name is empty")
	}
	errprefix := fmt.Sprintf("[Store: %s] ", config.Name)
	validstoretypes := []string{"IMAP", "Maildir"}
	if !StringInSlice(config.StoreType, validstoretypes) {
		return fmt.Errorf(errprefix+"Wrong store type: \"%s\". Valid types are: %s", config.StoreType, validstoretypes)
	}
	switch config.StoreType {
	case "IMAP":
		if config.Host == "" {
			return fmt.Errorf(errprefix + "host option is empty")
		}
		if config.Username == "" {
			return fmt.Errorf(errprefix + "username option is empty")
		}
		if config.Password == "" {
			return fmt.Errorf(errprefix + "password option is empty")
		}
		if config.Tls && config.Starttls {
			return fmt.Errorf(errprefix + "Both tls and starttls enabled. Only one of them is permitted.")
		}
	case "Maildir":
		if config.Maildir == "" {
			return fmt.Errorf(errprefix + "maildir option is empty")
		}

		validuidmappings := []string{"files", "db"}
		if !StringInSlice(config.UIDMapping, validuidmappings) {
			return fmt.Errorf(errprefix+"Wrong uidmapping: \"%s\". Valid uidmappings are: %s", config.UIDMapping, validuidmappings)
		}

		if config.UIDMapping == "db" {
			return fmt.Errorf(errprefix + "UIDmapping of type \"db\" not yet implemented")

		}

		validseparators := []rune{'.', '/'}
		if !RuneInSlice(config.Separator, validseparators) {
			return fmt.Errorf(errprefix+"Wrong uidmapping: \"%s\". Valid uidmappings are: %s", config.UIDMapping, validuidmappings)
		}

	}
	return
}

func VerifySyncGroupConfig(globalconfig *Config, config *SyncgroupConfig) (err error) {
	logger := log.GetLogger(fmt.Sprintf("%s", "config"), "debug")
	logger.Debugf("VerifySyncGroupConfig")

	if config.Name == "" {
		return fmt.Errorf("Syncgroup name is empty")
	}
	errprefix := fmt.Sprintf("[Syncgroup: %s] ", config.Name)

	if len(config.Stores) != 2 {
		err := fmt.Errorf(errprefix + "Wrong number of stores")
		logger.Debug(err)
		return err
	}

	validdeletemodes := []string{"expunge", "flag", "trash", "none"}
	if !StringInSlice(config.Deletemode, validdeletemodes) {
		return fmt.Errorf(errprefix+"Wrong deletemode: \"%s\". Valid modes are: %s", config.Deletemode, validdeletemodes)
	}
	if config.Deletemode == "trash" {
		return fmt.Errorf(errprefix + "deletemode of type \"trash\" not yet implemented")
	}

	// verify duration
	if int64(config.SyncInterval.Duration) < 0 {
		return fmt.Errorf(errprefix + "syncinterval must be positive.")
	}
	return
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func RuneInSlice(a rune, list []rune) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
