import (
  "log"
  "fmt"
  "net"
  "os"
  "strings"
  "strconv"

  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/extras"

  irc "gopkg.in/irc.v1"
)

// Returns an absolute Skylink URI to the established connection
func (r *Root) DialConnImpl(config *DialConfig) string {
  endpoint := fmt.Sprintf("%s:%s", config.Hostname, config.Port)

  firstMsg := &Message{
    Command: "LOG",
    Params: "Dialing " + endpoint + " over TCP...",
  }

  // Create the connection holder
  conn := &Connection{
    Config: config,
    State: "Pending",

    History: inmem.NewFolder("history"),
    HistoryHorizon: "0",
    HistoryLatest: "0",

    out: make(chan *Message),
  }
  conn.History.Put("0", firstMsg)

  // Helper to store messages
  addMsg := func (msg *Message) {
    i, _ := strconv.Atoi(conn.HistoryLatest)
    nextSeq := strconv.Itoa(i + 1)
    conn.History.Put(nextSeq, msg)
    conn.HistoryLatest = nextSeq
  }

  // Configure IRC library as needed
  conf := irc.ClientConfig{
    Nick: config.Nickname,
    Pass: config.Password,
    User: config.Username,
    Name: config.FullName,
    Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {

      // Add inbound messages to the history
      msg := &Message{
        Command: m.Command,
        Params: strings.Join(m.Params, "|"),
      }
      if m.Prefix != nil {
        msg.PrefixName = m.Prefix.Name
        msg.PrefixUser = m.Prefix.User
        msg.PrefixHost = m.Prefix.Host
      }
      addMsg(msg)

      /*if m.Command == "001" {
        // 001 is a welcome event, so we join channels there
        conn.State = "Ready"
        for _, name := range strings.Split(config.Channels, ",") {
          c.WriteMessage(&irc.Message{
            Command: "JOIN",
            Params: []string{name},
          })
        }
      } else if m.Command == "PRIVMSG" && m.FromChannel() {
        // Create a handler on all messages.
        c.WriteMessage(&irc.Message{
          Command: "PRIVMSG",
          Params: []string{
            m.Params[0],
            m.Trailing(),
          },
        })
      }*/
    }),
  }

  // Establish the network connection
  log.Println("Connecting to", endpoint, "using", config)
  netConn, err := net.Dial("tcp", endpoint)
  if err != nil {
    log.Println("Failed to dial", endpoint, err)
    return "Err! " + err.Error()
  }

  addMsg(&Message{
    Command: "LOG",
    Params: "Connection established.",
  })
  conn.State = "Ready"

  // Record username info in identd server
  if config.Identd == "" {
    config.Identd = "dialer"
  }
  identdRPC("add " + config.Identd + " " +
            strings.Split(netConn.LocalAddr().String(),":")[1] + " " +
            strings.Split(netConn.RemoteAddr().String(),":")[1])

  // Create the client
  conn.svc = irc.NewClient(netConn, conf)

  // Fire it up
  go func() {
    if err := conn.svc.Run(); err != nil {
      log.Println("Failed to run client:", err)
      conn.State = "Failed: " + err.Error()
    }

    close(conn.out)
  }()

  // Start outbound pump
  go func() {
    for msg := range conn.out {
      msg.PrefixName = conn.svc.CurrentNick()
      addMsg(msg)

      conn.svc.WriteMessage(&irc.Message{
        Command: msg.Command,
        Params: strings.Split(msg.Params, "|"),
      })
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

func identdRPC(line string) error {
  conn, err := net.Dial("tcp", "identd-rpc:11333")
  if err != nil {
    log.Println("Failed to dial identd rpc:", err)
    return err
  }

  _, err = conn.Write([]byte(line + "\n"))
  if err != nil {
    log.Println("Write to identd rpc failed:", err)
    return err
  }

  conn.Close()
  return nil
}
