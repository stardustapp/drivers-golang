func OpenConnectionImpl(opts *ConnectionOptions) *Connection {
  return &Connection{
    Options: opts,
  }
}