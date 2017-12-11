func (c *Connection) SendMessageImpl(m *Message) string {
  c.sendMutex.Lock()
  defer c.sendMutex.Unlock()

  if c.State.Get() == "Ready" {
    c.out <- m
    return "Ok"
  } else {
    return "Failed: connection is " + c.State.Get()
  }
}