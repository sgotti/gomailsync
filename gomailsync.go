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

package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/sgotti/gomailsync/config"
	"github.com/sgotti/gomailsync/log"
	"github.com/sgotti/gomailsync/mailsync"
	"os"
	"os/user"
	"path/filepath"
)

var opts struct {
	Configfile    string   `short:"c" long:"config" description:"Config file location. Default: ~/.gomailsyncrc"`
	Debug         bool     `short:"d" long:"debug" description:"Enable full debug logs. Overrides log levels in configuration file"`
	DryRun        bool     `short:"n" long:"dryrun" description:"Do not execute sync actions but just log what will be done"`
	List          bool     `short:"l" long:"list" description:"List stores infos and then exit"`
	SyncgroupList []string `short:"s" long:"syncgroup" description:"Limit the syncgroups to the specified. Use this option multiple times to specify multiple syncgroups."`
}

func main() {
	logger := log.GetLogger(fmt.Sprintf("%s", "main"), "info")
	u, err := user.Current()
	if err != nil {
		logger.Errorf("Cannot determine current user")
		os.Exit(1)
	}

	var parser = flags.NewParser(&opts, flags.Default)

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}

	if opts.Configfile == "" {
		opts.Configfile = filepath.Join(u.HomeDir, ".gomailsyncrc")
	}

	globalconfig, err := config.ParseConfig(opts.Configfile)
	if err != nil {
		logger.Errorf("Error parsing config file: %s", err)
		os.Exit(1)
	}

	err = config.VerifyConfig(globalconfig)
	if err != nil {
		logger.Errorf("Error parsing config file: %s", err)
		os.Exit(1)
	}

	if opts.Debug {
		globalconfig.LogLevel = "debug"
		globalconfig.DebugImap = true
	}

	if _, err := log.LogLevelToPriority(globalconfig.LogLevel); err != nil {
		logger.Errorf("Error: %s", err)
		os.Exit(1)
	}

	err = mailsync.MkdirIfNotExists(globalconfig.Metadatadir)
	if err != nil {
		logger.Errorf("Error: %s", err)
		os.Exit(1)
	}

	var count int = 0
	c := make(chan error)
	for _, syncgroupconf := range globalconfig.Syncgroups {
		if opts.SyncgroupList != nil {
			ok := false
			for _, s := range opts.SyncgroupList {
				if syncgroupconf.Name == s {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		syncgroup, err := mailsync.NewSyncgroup(globalconfig, syncgroupconf, opts.DryRun)
		if err != nil {
			logger.Errorf("Error creating syncgroup \"%s\": %s", syncgroupconf.Name, err)
			continue
		}

		if opts.List {
			syncgroup.List()
		} else {
			go syncgroup.SyncWrapper(-1, c)
			count++
		}

	}

	for count > 0 {
		err := <-c
		logger.Println("Sync exited:", err)
		count--
	}
}
