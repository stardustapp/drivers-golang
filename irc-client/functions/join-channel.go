func (c *Channel) JoinChannelImpl() {
  c.Connection.svc.Join(c.ChanName)

  c.scrollback = append(c.scrollback,
                        "Channel joined")
}