import ircevent "github.com/thoj/go-ircevent"
import "log"
import "fmt"

func (r *Root) OpenConnectionImpl(opts *ConnectionOptions) *Connection {
  token := fmt.Sprintf("%s.%s.%s.%s.%s", opts.Hostname, opts.Port, opts.Nickname, opts.Username)
  if r.conns == nil {
    r.conns = make(map[string]*Connection)
  }
  if conn, ok := r.conns[token]; ok {
    return conn
  }

  log.Println("connecting to", opts.Hostname, "as", opts.Nickname)

  ircobj := ircevent.IRC(opts.Nickname, opts.Username)
  ircobj.Debug = true
  ircobj.VerboseCallbackHandler = true

  ircobj.UseTLS = false
  ircobj.Password = opts.Password
  err := ircobj.Connect(opts.Hostname + ":" + opts.Port)

  conn := &Connection{
    Options: opts,
    svc: ircobj,
    channels: make(map[string]*Channel),
  }

  ircobj.AddCallback("001", func(e *ircevent.Event) {
    for c, _ := range conn.channels {
      ircobj.Join(c)
    }
    conn.IsConnected = "yes"
  })
  ircobj.AddCallback("PRIVMSG", func(e *ircevent.Event) {
    if channel, ok := conn.channels[e.Arguments[0]]; ok {
      msg := "<" + e.Nick + "> " + e.Message()
      channel.scrollback = append(channel.scrollback, msg)
    }
  })

  if err != nil {
    log.Println("Err", err)
    return nil
  }
  go ircobj.Loop()

  r.conns[token] = conn
  return conn
}

  /*
  ircobj.SendRaw("<string>") //sends string to server. Adds \r\n
  ircobj.SendRawf("<formatstring>", ...) //sends formatted string to server.n
  ircobj.Join("<#channel> [password]")
  ircobj.Nick("newnick")
  ircobj.Privmsg("<nickname | #channel>", "msg") // sends a message to either a certain nick or a channel
  ircobj.Privmsgf(<nickname | #channel>, "<formatstring>", ...)
  ircobj.Notice("<nickname | #channel>", "msg")
  ircobj.Noticef("<nickname | #channel>", "<formatstring>", ...)
  */
