import (
  "os"

  "github.com/stardustapp/core/extras"
  "github.com/stardustapp/core/inmem"

  "github.com/unixpickle/fbmsgr"
)

func (r *Root) DialImpl(config *DialConfig) *Session {
  session := &Session{
    Username: config.Username,
    State: "Pending",

    Threads: inmem.NewFolder("threads"),
    Participants: inmem.NewFolder("participants"),
  }

  // Store a session reference
  if r.Sessions == nil {
    // TODO: this should be made already
    r.Sessions = inmem.NewFolder("sessions")
  }
  sessionId := extras.GenerateId()
  if ok := r.Sessions.Put(sessionId, session); !ok {
    session.State = "Failed: Session store rejected us :("
    return session
  }

  // TODO: secure per-user credential story
  if svc, err := fbmsgr.Auth(config.Username, os.Getenv(config.Password)); err != nil {
    session.State = "Failed: During auth, " + err.Error()
  } else {
    session.svc = svc
    session.State = "Ready"

    go session.ListThreadsImpl()
  }

  return session
}