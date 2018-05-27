import (
  "log"
  "fmt"
  "time"
  "strconv"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/extras"
  "github.com/stardustapp/core/toolbox"

  "github.com/nlopes/slack"
)

func buildArrayFolder(in ...string) base.Folder {
  folder := inmem.NewFolder("array")
  for idx, str := range in {
    folder.Put(strconv.Itoa(idx+1), inmem.NewString("", str))
  }
  return folder
}

// Returns an absolute Skylink URI to the established connection
func (r *Root) DialSessionImpl(token string) string {

  // Create the connection holder
  conn := &Connection{
    State: toolbox.NewReactiveString("state", "Pending"),

    History: inmem.NewFolder("history"),
    HistoryHorizon: "0",
    HistoryLatest: toolbox.NewReactiveString("history-latest", "0"),
  }
  conn.History.Put("0", &Event{
    Type: "dialer",
    Params: inmem.NewFolderOf(
      "params", inmem.NewString("log", "Starting Slack API client...")),
    Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
  })

  // Helper to store event
  appendEvent := func (msg *Event) {
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

  // Create the protocol client
  conn.svc = slack.New(token)
  conn.svc.SetDebug(true)

  conn.rtm = conn.svc.NewRTM()
  go conn.rtm.ManageConnection()

  // Fire it up
  go func() {
    for msg := range conn.rtm.IncomingEvents {
      params := inmem.NewFolderOf("params")

      switch ev := msg.Data.(type) {
      case *slack.HelloEvent:

      case *slack.ConnectedEvent:
        // TODO: user prefs, users/channels/groups/bots/ims
        params.Put("info", inmem.NewFolderOf(
          "info",
          inmem.NewString("url", ev.Info.URL),
          inmem.NewFolderOf("user", inmem.NewFolderOf(
            "user",
            inmem.NewString("id", ev.Info.User.ID),
            inmem.NewString("name", ev.Info.User.Name),
            inmem.NewString("created", ev.Info.User.Created.Time().Format(time.RFC3339Nano)),
            inmem.NewString("manual presence", ev.Info.User.ManualPresence),
          )),
          inmem.NewFolderOf("team", inmem.NewFolderOf(
            "team",
            inmem.NewString("id", ev.Info.Team.ID),
            inmem.NewString("name", ev.Info.Team.Name),
            inmem.NewString("domain", ev.Info.Team.Domain),
          )),
        ))
        params.Put("connection count", inmem.NewString("connection count", strconv.Itoa(ev.ConnectionCount)))

      case *slack.MessageEvent:
        if ev.Type != "" {
          params.Put("type", inmem.NewString("type", ev.Type))
        }
        if ev.Channel != "" {
          params.Put("channel", inmem.NewString("channel", ev.Channel))
        }
        if ev.User != "" {
          params.Put("user", inmem.NewString("user", ev.User))
        }
        if ev.Text != "" {
          params.Put("text", inmem.NewString("text", ev.Text))
        }
        if ev.Timestamp != "" {
          params.Put("timestamp", inmem.NewString("timestamp", ev.Timestamp))
        }
        if ev.ThreadTimestamp != "" {
          params.Put("thread timestamp", inmem.NewString("thread timestamp", ev.ThreadTimestamp))
        }
        if ev.IsStarred == true {
          params.Put("is starred", inmem.NewString("is starred", "yes"))
        }
        if ev.LastRead != "" {
          params.Put("last read", inmem.NewString("last read", ev.LastRead))
        }
        if ev.SubType != "" {
          params.Put("sub type", inmem.NewString("sub type", ev.SubType))
        }

      case *slack.PresenceChangeEvent:
        params.Put("type", inmem.NewString("type", ev.Type))
        params.Put("presence", inmem.NewString("presence", ev.Presence))
        params.Put("user", inmem.NewString("user", ev.User))

      case *slack.LatencyReport:
        params.Put("latency", inmem.NewString("latency", ev.Value.String()))

      case *slack.RTMError:
        params.Put("code", inmem.NewString("code", strconv.Itoa(ev.Code)))
        params.Put("message", inmem.NewString("message", ev.Msg))

      case *slack.InvalidAuthEvent:
        fmt.Printf("Invalid credentials")
        go conn.rtm.Disconnect()
        return

      default:
        // Add inbound messages to the history
        appendEvent(&Event{
          Type: msg.Type,
          Params: params,
          Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        })

        // Ignore other events..
        // fmt.Printf("Unexpected: %v\n", msg.Data)
      }
    }

    conn.State.Set("Closed")
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
  log.Println("Given wire URI:", sessionUri)

  return sessionUri
}
