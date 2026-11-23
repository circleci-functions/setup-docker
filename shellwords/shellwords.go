// Package shellwords splits a string into words like a POSIX shell would.
package shellwords

import (
	"fmt"
	"strings"
)

// shellParser holds state for Split.
type shellParser struct {
	cur      strings.Builder
	inSingle bool
	inDouble bool
	escape   bool
}

func (p *shellParser) consumeByte(c byte) (flush bool) {
	if p.escape {
		p.cur.WriteByte(c)
		p.escape = false
		return false
	}
	switch {
	case c == '\\' && !p.inSingle:
		p.escape = true
	case c == '\'' && !p.inDouble:
		p.inSingle = !p.inSingle
	case c == '"' && !p.inSingle:
		p.inDouble = !p.inDouble
	case isShellSpace(c) && !p.inSingle && !p.inDouble:
		return true
	default:
		p.cur.WriteByte(c)
	}
	return false
}

func (p *shellParser) endError() error {
	switch {
	case p.inSingle:
		return fmt.Errorf("unterminated single quote")
	case p.inDouble:
		return fmt.Errorf("unterminated double quote")
	case p.escape:
		return fmt.Errorf("trailing backslash")
	}
	return nil
}

func isShellSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n'
}

// Split splits s like a POSIX shell, supporting single/double quotes
// and backslash escapes. It never evaluates variables or runs subshells.
func Split(s string) ([]string, error) {
	var words []string
	p := &shellParser{}
	for i := 0; i < len(s); i++ {
		if p.consumeByte(s[i]) && p.cur.Len() > 0 {
			words = append(words, p.cur.String())
			p.cur.Reset()
		}
	}
	if err := p.endError(); err != nil {
		return nil, err
	}
	if p.cur.Len() > 0 {
		words = append(words, p.cur.String())
	}
	return words, nil
}
