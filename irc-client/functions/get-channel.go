package driver

func (c *Connection) GetChannelImpl(chanName string) *Channel {
	if channel, ok := c.channels[chanName]; ok {
		return channel
	}

	channel := &Channel{
		Connection: c,
		ChanName:   chanName,
		scrollback: []string{"Channel opened"},
	}

	c.channels[chanName] = channel
	return channel
}
