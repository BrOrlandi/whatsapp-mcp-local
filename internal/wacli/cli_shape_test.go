package wacli

import (
	"reflect"
	"testing"
)

func TestCommandShapeKeepsNamesNotValues(t *testing.T) {
	for _, c := range []struct {
		args  []string
		cmd   string
		flags []string
	}{
		{[]string{"--read-only", "auth", "status"}, "auth status", []string{"--read-only"}},
		{[]string{"chats", "mark-read", "--chat", "5511987654321@s.whatsapp.net"}, "chats mark-read", []string{"--chat"}},
		{[]string{"send", "text", "--to", "abc", "--message", "oi tudo bem"}, "send text", []string{"--to", "--message"}},
		{[]string{"history", "backfill", "--chat=x@g.us", "--count", "50"}, "history backfill", []string{"--chat", "--count"}},
	} {
		cmd, flags := commandShape(c.args)
		if cmd != c.cmd || !reflect.DeepEqual(flags, c.flags) {
			t.Errorf("commandShape(%q) = %q %q, want %q %q", c.args, cmd, flags, c.cmd, c.flags)
		}
	}
}
