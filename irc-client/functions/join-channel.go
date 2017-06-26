func (c *Connection) JoinChannelImpl(opts *JoinOptions) string {
  c.svc.Join(opts.Channel + " :" + opts.Password)
  return "joined!"
}