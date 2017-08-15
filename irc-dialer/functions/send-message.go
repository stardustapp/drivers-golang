func (c *Connection) SendMessageImpl(m *Message) string {
  c.sendMutex.Lock()
  defer c.sendMutex.Unlock()

  if c.State == "Ready" {
    c.out <- m
    return "ok"
  } else {
    return "failed: connection is " + c.State
  }
}