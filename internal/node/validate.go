package node

import (
	"regexp"
	"strings"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var repoRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validID(s string) bool { return idRE.MatchString(s) }
func validRepo(s string) bool {
	if !repoRE.MatchString(s) || strings.Contains(s, "..") {
		return false
	}
	return true
}
func isHexLen(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') && !(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
