import ircevent "github.com/thoj/go-ircevent"
import "log"

func OpenConnectionImpl(opts *ConnectionOptions) *Connection {
  log.Println("connecting")

  ircobj := ircevent.IRC(opts.Nickname, opts.Username)
  ircobj.Debug = true
  ircobj.VerboseCallbackHandler = true

  ircobj.AddCallback("001", func(e *ircevent.Event) {
    ircobj.Join("##stardust")
  })

  ircobj.UseTLS = true
  ircobj.Password = opts.Password
  err := ircobj.Connect(opts.Hostname + ":" + opts.Port)

  if err != nil {
    log.Println("Err", err)
    return nil
  }
  go ircobj.Loop()

  return &Connection{
    Options: opts,
    svc: ircobj,
  }
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
