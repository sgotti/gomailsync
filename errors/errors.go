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

package errors

import (
	"fmt"
	"github.com/satori/go.uuid"
)

type Error struct {
	prefix string
	uuid   uuid.UUID
}

type errorError struct {
	prefix string
	err    error
	uuid   uuid.UUID
}

func New(prefix string) *Error {
	return &Error{prefix, uuid.NewV1()}
}

func (e *Error) E(err error) error {
	if err == nil {
		return nil
	}
	if ee, ok := err.(*errorError); ok {
		if ee.uuid == e.uuid {
			return &errorError{e.prefix, ee.err, e.uuid}
		}
	}
	return &errorError{e.prefix, err, e.uuid}
}

func (e *errorError) Error() string {
	return fmt.Sprintf("[%s] %s", e.prefix, e.err.Error())
}
