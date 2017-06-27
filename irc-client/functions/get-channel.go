func (c *Connection) GetChannelImpl(chanName string) *Channel {
  return &Channel{
    Connection: c,
    ChanName: chanName,
  }
}