package naming

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewClaudeSessionID returns a random RFC 4122 version-4 UUID string, used as a
// stable Claude Code session identifier for one fleet session. It panics only
// if the system CSPRNG is unavailable, which is unrecoverable.
func NewClaudeSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("naming: reading random bytes: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	s := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}
