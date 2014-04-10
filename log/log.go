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

package log

import (
	"fmt"
	golog "github.com/coreos/go-log/log"
	"os"
)

var defaultSync = golog.WriterSink(os.Stderr,
	"%s %s: %s[%d] [%s] %s\n",
	[]string{"time", "priority", "executable", "pid", "prefix", "message"})

type Logger struct {
	*golog.Logger
}

func GetLogger(prefix string, loglevel string) *Logger {
	pri, _ := LogLevelToPriority(loglevel)
	logger := &Logger{golog.New(prefix, false, &PriorityFilter{
		pri,
		defaultSync,
	})}
	return logger
}

type PriorityFilter struct {
	priority golog.Priority
	target   golog.Sink
}

func (filter *PriorityFilter) Log(fields golog.Fields) {
	// lower priority values indicate more important messages
	if fields["priority"].(golog.Priority) <= filter.priority {
		filter.target.Log(fields)
	}
}

var (
	LogLevelMap = map[string]golog.Priority{
		"error": golog.PriErr,
		"info":  golog.PriInfo,
		"debug": golog.PriDebug,
	}
)

func LogLevelToPriority(loglevel string) (golog.Priority, error) {
	if l, ok := LogLevelMap[loglevel]; ok {
		return l, nil
	}
	err := fmt.Errorf("Wrong log level: %s", loglevel)
	return 0, err
}
