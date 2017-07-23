import (
  "log"
  "fmt"
  "net"
  "os"

  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/extras"

  irc "gopkg.in/irc.v1"
)

// Returns an absolute Skylink URI to the established connection
func (r *Root) DialConnImpl(config *DialConfig) string {

  // Create the connection holder
  conn := &Connection{
    Config: config,
    State: "Pending",
    Channels: inmem.NewFolder("channels"), // TODO
  }

  // Configure IRC library as needed
  conf := irc.ClientConfig{
    Nick: config.Nickname,
    Pass: config.Password,
    User: config.Username,
    Name: config.FullName,
    Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {
      if m.Command == "001" {
        // 001 is a welcome event, so we join channels there
        conn.State = "Ready"
        c.Write("JOIN #general")
      } else if m.Command == "PRIVMSG" && m.FromChannel() {
        // Create a handler on all messages.
        c.WriteMessage(&irc.Message{
          Command: "PRIVMSG",
          Params: []string{
            m.Params[0],
            m.Trailing(),
          },
        })
      }
    }),
  }

  // Establish the network connection
  endpoint := fmt.Sprintf("%s:%s", config.Hostname, config.Port)
  log.Println("Connecting to", endpoint, "using", config)
  netConn, err := net.Dial("tcp", endpoint)
  if err != nil {
    log.Println("Failed to dial", endpoint, err)
    return "Err! " + err.Error()
  }

  // TODO: identd
  // add dan-chat local-port remote-port
  // ports via strings.Split(conn.LocalAddr().String(),":")[1]

  // Create the client
  conn.svc = irc.NewClient(netConn, conf)

  // Fire it up
  go func() {
    if err := conn.svc.Run(); err != nil {
      log.Println("Failed to run client:", err)
      conn.State = "Failed: " + err.Error()
    }
  }()

  // TODO: this should be made already
  if r.Sessions == nil {
    r.Sessions = inmem.NewFolder("sessions")
  }

  // Store a session reference
  sessionId := extras.GenerateId()
  if ok := r.Sessions.Put(sessionId, conn); !ok {
    log.Println("Session store rejected us :(", err)
    return "Err! Couldn't store session"
  }

  // Return absolute URI to the created session
  name, err := os.Hostname()
  if err != nil {
    log.Println("Oops 1:", err)
    return "Err! no ip"
  }
  addrs, err := net.LookupHost(name)
  if err != nil {
    log.Println("Oops 2:", err)
    return "Err! no host"
  }
  if len(addrs) < 1 {
    log.Println("Oops 2:", err)
    return "Err! no host ip"
  }
  selfIp := addrs[0]

  return fmt.Sprintf("skylink+ws://%s:9234/pub/sessions/%s", selfIp, sessionId)
}
