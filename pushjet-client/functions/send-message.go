import pushjet "github.com/geir54/goPushJet"
import "strconv"

func (svc *Service) SendMessageImpl(msg *Message) string {
  level, _ := strconv.Atoi(msg.Level)
  err := pushjet.SendMessage(svc.Secret, msg.Text, msg.Title, level, msg.Link)

  if err != nil {
    return err.Error()
  } else {
    return "ok"
  }
}