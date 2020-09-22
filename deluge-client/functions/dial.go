package driver

import (
	"fmt"
	"log"

	"github.com/stardustapp/dustgo/lib/extras"
	"github.com/stardustapp/dustgo/lib/inmem"
	"github.com/stardustapp/dustgo/lib/toolbox"

	deluge "github.com/pyed/go-deluge"
)

func (r *Root) DialImpl(config *DialConfig) string {

	// Return absolute URI to the created session
	if r.Sessions == nil {
		// TODO: this should be made already
		r.Sessions = inmem.NewObscuredFolder("sessions")
	}

	sessionId := extras.GenerateId()
	sessionPath := fmt.Sprintf(":9234/pub/sessions/%s", sessionId)
	sessionUri, _ := toolbox.SelfURI(sessionPath)

	// make the client
	svc, err := deluge.New(config.URL, config.Password)
	if err != nil {
		log.Println("WARN: deluge client failed to launch,", err)
		return "ERR"
	}
	session := &Client{svc}
	log.Printf("built session %+v", session)

	if ok := r.Sessions.Put(sessionId, session); !ok {
		log.Println("WARN: Session store rejected us :(")
		return "ERR"
	}
	return sessionUri
}
