import (
  "crypto/tls"
  "log"
  "errors"
  "fmt"
  "net"
  "time"
  "os"
  "os/signal"
  "strings"
  "strconv"
  "sync"
  "syscall"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/extras"
  "github.com/stardustapp/core/toolbox"

  irc "gopkg.in/irc.v1"
)

// set up a global process-shutdown signal
var shutdownChan chan struct{}
var isShuttingDown bool
var wgWires sync.WaitGroup
func init() {
  shutdownChan = make(chan struct{})
  c := make(chan os.Signal, 2)
  signal.Notify(c, os.Interrupt, syscall.SIGTERM)

  // start waiting for interupt signals
  go func() {
    <-c
    isShuttingDown = true
    log.Println("WARN: Received Interrupt - quitting all sockets")
    close(shutdownChan)

    go func() {
      <-c // let user interactively fast-bail
      log.Println("FATAL: Received Interrupt AGAIN - suiciding")
      os.Exit(1)
    }()

    log.Println("Waiting for no running wires...")
    wgWires.Wait()
    log.Println("Sleeping 5s, allowing logs to settle...")
    time.Sleep(5 * time.Second)
    log.Println("Done! :) 'Til next time")
    os.Exit(0)
  }()
}

func buildArrayFolder(in ...string) base.Folder {
  folder := inmem.NewFolder("array")
  for idx, str := range in {
    folder.Put(strconv.Itoa(idx+1), inmem.NewString("", str))
  }
  return folder
}

// Returns an absolute Skylink URI to the established connection
func (r *Root) DialConnImpl(config *DialConfig) string {

  // Don't start up if the sky is falling
  if isShuttingDown {
    log.Println("warn: rejecting dial due to shutdown state")
    return "Err: This IRC modem is shutting down"
  }

  // Build the full endpoint
  endpoint := fmt.Sprintf("%s:%s", config.Hostname, config.Port)
  firstMsg := &Message{
    Command: "LOG",
    Params: buildArrayFolder("Dialing " + endpoint + " over TCP..."),
    Source: "dialer",
    Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
  }

  // Create the connection holder
  conn := &Connection{
    Config: config,
    State: toolbox.NewReactiveString("state", "Pending"),

    History: inmem.NewFolder("history"),
    HistoryHorizon: "0",
    HistoryLatest: toolbox.NewReactiveString("history-latest", "0"),

    out: make(chan *Message),
  }
  conn.History.Put("0", firstMsg)

  // Helper to store messages
  addMsg := func (msg *Message) {
    i, _ := strconv.Atoi(conn.HistoryLatest.Get())
    nextSeq := strconv.Itoa(i + 1)
    conn.History.Put(nextSeq, msg)
    conn.HistoryLatest.Set(nextSeq)

    // Trim old messages
    horizon, _ := strconv.Atoi(conn.HistoryHorizon)
    maxOld := i - 250
    for horizon < maxOld {
      conn.History.Put(strconv.Itoa(horizon), nil)
      horizon++
      conn.HistoryHorizon = strconv.Itoa(horizon)
    }
  }

  // Track our info for outbound packets
  var currentNick string

  // Configure IRC library as needed
  conf := irc.ClientConfig{
    Nick: config.Nickname,
    Pass: config.Password,
    User: config.Username,
    Name: config.FullName,
    Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {

      // Clean up CTCP stuff so everyone doesn't have to parse it manually.
      // TODO: the go-irc library does this but only for PRIVMSG
      // TODO: split the ctcp cmd from the ctcp args
      if m.Command == "NOTICE" {
        lastArg := m.Trailing()
        lastIdx := len(lastArg) - 1
        if lastIdx > 0 && lastArg[0] == '\x01' && lastArg[lastIdx] == '\x01' {
          m.Command = "CTCP_ANSWER"
          m.Params[len(m.Params)-1] = lastArg[1:lastIdx]
        }
      }

      // Track nickname - TODO: irc-app really should handle this
      if m.Command == "001" {
        currentNick = m.Params[0]
        log.Println("Changed nickname from", currentNick, "to", m.Params[0])
      }
      if m.Command == "NICK" {
        if m.Prefix.Name == currentNick && len(m.Params) > 0 {
          currentNick = m.Params[0]
        }
      }

      // Add inbound messages to the history
      msg := &Message{
        Command: m.Command,
        Params: buildArrayFolder(m.Params...),
        Source: "server",
        Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        Tags: inmem.NewFolder("tags"),
      }
      if m.Prefix != nil {
        msg.PrefixName = m.Prefix.Name
        msg.PrefixUser = m.Prefix.User
        msg.PrefixHost = m.Prefix.Host
      }
      for key, _ := range m.Tags {
        if val, ok := m.GetTag(key); ok {
          msg.Tags.Put(key, inmem.NewString(key, val))
        }
      }
      addMsg(msg)
    }),
  }

  // Establish the network connection
  log.Println("Connecting to TCP server at", endpoint)
  rawConn, err := net.Dial("tcp", endpoint)
  if err != nil {
    log.Println("Failed to dial", endpoint, err)
    conn.State.Set("Failed: Dial error")
    return "Err! " + err.Error()
  }
  var netConn net.Conn = rawConn

  // Record username info in identd server
  if config.Ident == "" {
    config.Ident = "dialer"
  }
  if localAddr, ok := netConn.LocalAddr().(*net.TCPAddr); ok {
    if remoteAddr, ok := netConn.RemoteAddr().(*net.TCPAddr); ok {
      identdRPC("add " + config.Ident + " " +
                strconv.Itoa(localAddr.Port) + " " +
                strconv.Itoa(remoteAddr.Port))
      // TODO: not behind NAT? also send localAddr.IP.String()
    }
  }

  // Perform TLS setup if desired
  if config.UseTLS == "yes" {
    addMsg(&Message{
      Command: "LOG",
      Params: buildArrayFolder("Performing TLS handshake..."),
      Source: "dialer",
      Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
    })

    // Extract hostname of endpoint
    colonPos := strings.LastIndex(endpoint, ":")
    if colonPos == -1 {
      colonPos = len(endpoint)
    }
    hostname := endpoint[:colonPos]

    // Configure a TLS client
    log.Println("Starting TLS handshake with", endpoint)
    tlsConn := tls.Client(rawConn, &tls.Config{
      ServerName: hostname,
      NextProtos: []string{"irc"},
    })

    // Do a little dance
    if err := tlsConn.Handshake(); err != nil {
      log.Println("Failed to perform TLS handshake:", endpoint, err)
      conn.State.Set("Failed: TLS error")
      return "Err! " + err.Error()
    }
    netConn = tlsConn
  }

  // Record that the analog transport is configured
  addMsg(&Message{
    Command: "LOG",
    Params: buildArrayFolder("Connection established."),
    Source: "dialer",
    Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
  })
  conn.State.Set("Ready")

  // Create the protocol client
  conn.svc = irc.NewClient(netConn, conf)

  // Fire it up
  wgWires.Add(1)
  go func() {
    err := conn.svc.Run()
    if err == nil {
      err = errors.New("IRC client had no error")
    }
    log.Println("IRC client failed while running:", err)

    // We hit this when the client stops running, so record that
    addMsg(&Message{
      Command: "LOG",
      Params: buildArrayFolder("Connection closed: " + err.Error()),
      Source: "dialer",
      Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
    })

    // synchronize to prevent send-message from panicing
    conn.sendMutex.Lock()
    defer conn.sendMutex.Unlock()

    wgWires.Done()
    conn.State.Set("Closed")
    close(conn.out)
  }()

  // Also watch for process shutdown
  go func() {
    <-shutdownChan
    log.Println("Shutting down client", config.Nickname, "on", endpoint)

    // synchronize to prevent send-message from panicing
    conn.sendMutex.Lock()
    defer conn.sendMutex.Unlock()

    if conn.State.Get() == "Closed" {
      return // our IRC connection is already gone
    }

    // attempt to peacefully disconnect
    conn.out <- &Message{
      Command: "QUIT",
      Params: inmem.NewFolderOf("params", inmem.NewString(
        "1", "IRC modem is shutting down")),
    }

    conn.State.Set("Quitting")
  }()

  // Start outbound pump
  go func() {
    for msg := range conn.out {
      msg.PrefixName = currentNick
      msg.Source = "client"
      msg.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
      addMsg(msg)

      // pull native params out of param folder
      var params []string
      if msg.Params != nil {
        params = make([]string, len(msg.Params.Children()))
        for _, name := range msg.Params.Children() {
          id, _ := strconv.Atoi(name)
          if ent, ok := msg.Params.Fetch(name); ok && id > 0 && id <= len(params) {
            params[id-1] = ent.(base.String).Get()
          }
        }
      }

      // pull native tags out too
      var tags map[string]irc.TagValue
      if msg.Tags != nil {
        tags = make(map[string]irc.TagValue, len(msg.Tags.Children()))
        for _, name := range msg.Tags.Children() {
          if ent, ok := msg.Tags.Fetch(name); ok {
            tags[name] = irc.TagValue(ent.(base.String).Get())
          }
        }
      }

      // encode CTCP payloads and answers
      command := msg.Command
      if command == "CTCP" || command == "CTCP_ANSWER" {
        var payload string
        if len(params) > 2 {
          payload = "\x01" + params[1] + " " + params[2] + "\x01"
        } else if len(params) == 2 {
          payload = "\x01" + params[1] + "\x01"
        }
        params = []string{params[0], payload}

        if command == "CTCP_ANSWER" {
          command = "NOTICE"
        } else {
          command = "PRIVMSG"
        }
      }

      err := conn.svc.WriteMessage(&irc.Message{
        Command: command,
        Params: params,
        Tags: tags,
      })
      if err != nil {
        // TODO: do something about these errors
        log.Println("Unexpected error writing IRC payload:", err)
      }
    }
  }()

  // TODO: this should be made already
  if r.Sessions == nil {
    r.Sessions = inmem.NewObscuredFolder("sessions")
  }

  // Store a session reference
  sessionId := extras.GenerateId()
  if ok := r.Sessions.Put(sessionId, conn); !ok {
    log.Println("Session store rejected us :(")
    return "Err! Couldn't store session"
  }

  // Return absolute URI to the created session
  sessionPath := fmt.Sprintf(":9234/pub/sessions/%s", sessionId)
  sessionUri, _ := toolbox.SelfURI(sessionPath)

  // TODO
  log.Println("Raw wire URI:", sessionUri)
  sessionUri = strings.Replace(sessionUri, "172.31.28.120:", "modem2.devmode.cloud:2", 1)
  log.Println("Given wire URI:", sessionUri)

  return sessionUri
}

func identdRPC(line string) error {
  conn, err := net.Dial("tcp", "identd-rpc:1133")
  if err != nil {
    log.Println("Failed to dial identd rpc:", err)
    return err
  }

  _, err = conn.Write([]byte(line + "\n"))
  if err != nil {
    log.Println("Write to identd rpc failed:", err)
    return err
  }

  log.Println("Issued identd RPC command:", line)

  conn.Close()
  return nil
}
