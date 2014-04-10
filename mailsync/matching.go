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
	"fmt"
	"regexp"
	"strings"
)

type RegexpPattern struct {
	not bool
	re  *regexp.Regexp
}

func ValidatePattern(pattern string) bool {
	if _, err := RegexpFromPattern(pattern); err != nil {
		return false
	}
	return true
}

func RegexpFromPattern(pattern string) (rp *RegexpPattern, err error) {
	if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "!/") {
		return nil, fmt.Errorf("pattern doesn't starts with \"/\" or \"!/\"")
	}

	if !strings.HasSuffix(pattern, "/") {
		return nil, fmt.Errorf("pattern doesn't ends with \"/\"")
	}

	res := pattern
	not := false
	if strings.HasPrefix(res, "!") {
		not = true
		res = strings.TrimPrefix(res, "!")
	}

	if strings.HasPrefix(res, "/") {
		res = strings.TrimPrefix(res, "/")
	}

	if strings.HasSuffix(res, "/") {
		res = strings.TrimSuffix(res, "/")
	}

	re, err := regexp.Compile(res)
	if err != nil {
		return nil, fmt.Errorf("re: \"%s\" wrong regexp: %s", err)
	}

	rp = &RegexpPattern{not: not, re: re}

	return rp, nil
}
