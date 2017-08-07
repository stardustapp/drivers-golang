import "strings"
import "fmt"

func (s *Session) ListThreadsImpl() string {
  var idx int
  for {
    listing, err := s.svc.Threads(idx, 20)
    if err != nil {
      return err.Error()
    }
    for _, t := range listing.Threads {
      otherUserId := ""
      if t.OtherUserFBID != nil {
        otherUserId = *t.OtherUserFBID
      }

      ent := &Thread{
        ThreadName: t.Name,
        ThreadID: t.ThreadID,
        FbID: t.ThreadFBID,
        OtherUserID: otherUserId,
        ParticipantIds: strings.Join(t.Participants, "|"),
        Snippet: t.Snippet,
        SnippetSenderID: t.SnippetSender,
        UnreadCount: fmt.Sprintf("%v", t.UnreadCount),
        MessageCount: fmt.Sprintf("%v", t.MessageCount),
        Timestamp: fmt.Sprintf("%v", t.Timestamp),
        ServerTimestamp: fmt.Sprintf("%v", t.ServerTimestamp),
      }
      s.Threads.Put(ent.ThreadID, ent)
    }
    for _, p := range listing.Participants {
      ent := &Participant{
        ID: p.ID,
        FbID: p.FBID,
        Gender: fmt.Sprintf("%v", p.Gender),
        Href: p.HREF,
        ImageSrc: p.ImageSrc,
        BigImageSrc: p.BigImageSrc,
        ParticipantName: p.Name,
        ShortName: p.ShortName,
      }
      s.Participants.Put(ent.ID, ent)
    }
    if len(listing.Threads) < 20 {
      break
    }
    idx += len(listing.Threads)
  }

  return "ok"
}