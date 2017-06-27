func (c *Channel) SendMessageImpl(message string) {
  c.Connection.svc.Privmsg(c.ChanName, message)
}