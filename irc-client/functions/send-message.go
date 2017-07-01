func (c *Channel) SendMessageImpl(message string) {
  c.Connection.svc.Privmsg(c.ChanName, message)

  msg := "<" + c.Connection.Options.Nickname + "> " + message
  c.scrollback = append(c.scrollback, msg)
}