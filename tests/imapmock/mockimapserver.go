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

// Idea and some code took from go-imap (github.com/mxk/go-imap/mock)
package imapmock

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func newLocalListener() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("mockimapserver: failed to listen on a port: %v", err))
		}
	}
	return l
}

var cr = byte('\r')
var lf = byte('\n')

// Line termination.
var crlf = []byte{cr, lf}

type ScriptFunc func(s *Server) error

type (
	Send []byte
	Recv []byte
)

type Server struct {
	*testing.T
	l         net.Listener
	greetings string
}

type Connection struct {
	*testing.T

	s  *Server
	ch <-chan interface{}
	cn net.Conn

	rw *bufio.ReadWriter

	tags []string
}

func NewMockImapServer(T *testing.T, greetings string) *Server {
	return &Server{
		T:         T,
		l:         newLocalListener(),
		greetings: greetings,
	}
}

func (s *Server) GetServerAddress() net.Addr {
	return s.l.Addr()
}

func (s *Server) WaitConnection() (conn *Connection, err error) {
	// Wait for a connection.
	cn, err := s.l.Accept()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	rw := bufio.NewReadWriter(bufio.NewReader(cn), bufio.NewWriter(cn))

	conn = &Connection{
		T:  s.T,
		s:  s,
		cn: cn,
		rw: rw,
	}
	conn.write([]byte(s.greetings))

	return conn, nil
}

func (c *Connection) Script(script ...interface{}) {
	select {
	case <-c.ch:
	default:
		if c.ch != nil {
			c.Fatalf(cl("Script() called while another script is active"))
		}
	}
	ch := make(chan interface{}, 1)
	c.ch = ch
	// Clean tags history
	c.tags = make([]string, 0)
	go c.script(script, ch)
}

func (c *Connection) Check() {
	if err, ok := <-c.ch; err != nil {
		c.Errorf(cl("Check() S: %v"), err)
	} else if !ok {
		c.Errorf(cl("Check() called without an active script"))
	}
}

func (c *Connection) script(script []interface{}, ch chan<- interface{}) {
	defer func() { ch <- recover(); close(ch) }()
	for ln, v := range script {
		switch ln++; v := v.(type) {
		case string:
			if strings.HasPrefix(v, "S: ") {
				_, err := c.writeString(v[3:])
				c.flush(ln, v, err)
			} else if strings.HasPrefix(v, "C: ") {
				b, _, err := c.readLine()
				c.compare(ln, v[3:], string(b), err)
			} else {
				panicf(`[#%d] %+q must be prefixed with "S: " or "C: "`, ln, v)
			}
		case Send:
			_, err := c.write(v)
			c.flush(ln, v, err)
		case Recv:
			b := make([]byte, len(v))
			_, err := c.readFull(b)
			c.compare(ln, string(v), string(b), err)
		case ScriptFunc:
			c.run(ln, v)
		case func(s *Server) error:
			c.run(ln, v)
		default:
			panicf("[#%d] %T is not a valid script action", ln, v)
		}
	}
}

func (c *Connection) flush(ln int, v interface{}, err error) {
	if err == nil {
		err = c.rw.Flush()
	}
	if err != nil {
		panicf("[#%d] %+q write error: %v", ln, v, err)
	}
}

// compare panics if v != b or err != nil.
func (c *Connection) compare(ln int, v, b string, err error) {
	// Get the tag
	var tagidxstr string
	splitv := strings.SplitN(v, " ", 2)
	if strings.HasPrefix(splitv[0], "TAG") {
		tagidxstr = strings.TrimLeft(splitv[0], "TAG")
	}
	tagidx, err := strconv.Atoi(tagidxstr)

	// Extract tag from client data
	splitb := strings.SplitN(b, " ", 2)
	if splitb[0] != "*" && splitb[0] != "+" {
		if !contains(c.tags, splitb[0]) {
			c.tags = append(c.tags, splitb[0])
		}
	}
	idx := index(c.tags, splitb[0])
	if tagidx != idx {
		panicf("[#%d] wrong tag idx")
	}
	if splitv[1] != splitb[1] || err != nil {
		panicf("[#%d] expected %+q; got %+q (%v)", ln, v, b, err)
	}
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func index(s []string, e string) int {
	for i, a := range s {
		if a == e {
			return i
		}
	}
	return -1
}

func (c *Connection) run(ln int, v ScriptFunc) {
	if err := v(c.s); err != nil {
		panicf("[#%d] ScriptFunc error: %v", ln, err)
	}
}

func cl(s string) string {
	_, testFile, line, ok := runtime.Caller(2)
	if ok && strings.HasSuffix(testFile, "_test.go") {
		return fmt.Sprintf("%d: %s", line, s)
	}
	return s
}

func panicf(format string, v ...interface{}) {
	panic(fmt.Sprintf(format, v...))
}

func (c *Connection) writeString(s string) (n int, err error) {
	// Get the tag
	var data string
	var tagidxstr string
	split := strings.SplitN(s, " ", 2)
	if strings.HasPrefix(split[0], "TAG") {
		tagidxstr = strings.TrimLeft(split[0], "TAG")
		tagidx, _ := strconv.Atoi(tagidxstr)
		tag := c.tags[tagidx]
		data = tag + " " + split[1]
	} else {
		data = s
	}

	n, err = c.rw.Write([]byte(data))
	if err != nil {
		return
	}
	c.rw.Write(crlf)
	c.rw.Flush()
	return
}

func (c *Connection) write(data []byte) (n int, err error) {
	n, err = c.rw.Write(data)
	if err != nil {
		return
	}
	c.rw.Write(crlf)
	c.rw.Flush()
	return
}

func (c *Connection) readFull(data []byte) (n int, err error) {
	return io.ReadFull(c.rw, data)
}

func (c *Connection) readLine() (line []byte, isPrefix bool, err error) {
	return c.rw.ReadLine()
}

func (c *Connection) Close() error {
	return c.cn.Close()
}
