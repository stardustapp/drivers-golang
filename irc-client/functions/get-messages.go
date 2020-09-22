package driver

import "strings"

func (c *Channel) GetMessagesImpl() string {
	return strings.Join(c.scrollback, "\n")
}
