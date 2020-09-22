package driver

func (conn *Connection) SendMessageImpl(input *SendMessageInput) string {
	message := conn.rtm.NewOutgoingMessage(input.Body, input.Target)
	conn.rtm.SendMessage(message)
	return "Ok"
}
