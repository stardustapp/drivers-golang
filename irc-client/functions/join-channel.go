func (c *Channel) JoinChannelImpl(opts *JoinOptions) {
  c.Connection.svc.Join(c.ChanName + " :" + opts.Password)
}