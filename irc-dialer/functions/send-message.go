func (c *Connection) SendMessageImpl(m *Message) {
  c.out <- m
}