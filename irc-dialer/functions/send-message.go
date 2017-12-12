import "strings"
import "github.com/stardustapp/core/base"

const BAD_CHARS = "\r\n\000"

func (c *Connection) SendMessageImpl(msg *Message) string {
  c.sendMutex.Lock()
  defer c.sendMutex.Unlock()

  // check command, params, & tags for newlines and such
  if (strings.ContainsAny(msg.Command, BAD_CHARS)) {
    return "Failed: command contains invalid chars";
  }
  if msg.Params != nil {
    for _, name := range msg.Params.Children() {
      if ent, ok := msg.Params.Fetch(name); ok {
        if (strings.ContainsAny(ent.(base.String).Get(), BAD_CHARS)) {
          return "Failed: params contain invalid chars";
        }
      }
    }
  }
  if msg.Tags != nil {
    for _, name := range msg.Tags.Children() {
      if ent, ok := msg.Tags.Fetch(name); ok {
        if (strings.ContainsAny(ent.(base.String).Get(), BAD_CHARS)) {
          return "Failed: tags contain invalid chars";
        }
      }
    }
  }

  // Actually push the message to the outbound pump
  if c.State.Get() == "Ready" {
    c.out <- msg
    return "Ok"
  } else {
    return "Failed: connection is " + c.State.Get()
  }
}