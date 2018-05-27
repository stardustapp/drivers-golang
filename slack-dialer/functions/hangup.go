func (conn *Connection) HangupImpl() string {
  conn.State.Set("Hanging up")
  if err := conn.rtm.Disconnect(); err != nil {
    return "Failed to disconnect RTM: "+err.Error()
  }
  return "Ok"
}