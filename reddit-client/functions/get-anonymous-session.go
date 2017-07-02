import "github.com/jzelinskie/geddit"

func GetAnonymousSessionImpl() *Session {
  return &Session{
    svc: geddit.NewSession("stardust/0.1"),
  }
}