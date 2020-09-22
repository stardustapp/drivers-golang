package driver

import pushjet "github.com/geir54/goPushJet"
import "strconv"
import "log"

func (svc *Service) SendMessageImpl(msg *Message) string {
	level, _ := strconv.Atoi(msg.Level)
	err := pushjet.SendMessage(svc.Secret, msg.Text, msg.Title, level, msg.Link)

	if err != nil {
		log.Println(err.Error())
		return err.Error()
	} else {
		log.Println("ok")
		return "ok"
	}
}
