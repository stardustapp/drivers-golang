import "github.com/jzelinskie/geddit"
import "github.com/stardustapp/core/inmem"
import "strconv"
import "time"
import "log"

// Get reddit's default frontpage
func (s *Session) GetDefaultFrontpageImpl() *inmem.Channel {
  // Set listing options
  subOpts := geddit.ListingOptions{
    Limit: 10,
  }

  submissions, err := s.svc.DefaultFrontpage(geddit.DefaultPopularity, subOpts)
  if err != nil {
    panic("Error getting default frontpage: " + err.Error())
  }

  channel := inmem.NewBufferedChannel("submissions", len(submissions))
  log.Println("Generating", len(submissions), "submissions")
  for _, s := range submissions {

    bannedBy := ""
    if s.BannedBy != nil {
      bannedBy = *s.BannedBy
    }
    bools := map[bool]string{
      true: "yes",
      false: "no",
    }
    dateCreated := time.Unix(int64(s.DateCreated), 0)
    dateCreatedStr, _ := dateCreated.MarshalText()

    channel.Push(&Submission{
      Author: s.Author,
      Title: s.Title,
      URL: s.URL,
      Domain: s.Domain,
      Subreddit: s.Subreddit,
      SubredditID: s.SubredditID,
      FullID: s.FullID,
      ID: s.ID,
      Permalink: s.Permalink,
      Selftext: s.Selftext,
      ThumbnailURL: s.ThumbnailURL,

      DateCreated: string(dateCreatedStr),

      NumComments: strconv.Itoa(s.NumComments),
      Score: strconv.Itoa(s.Score),
      Ups: strconv.Itoa(s.Ups),
      Downs: strconv.Itoa(s.Downs),

      IsNsfw: bools[s.IsNSFW],
      IsSelf: bools[s.IsSelf],
      WasClicked: bools[s.WasClicked],
      IsSaved: bools[s.IsSaved],

      BannedBy: bannedBy,
    })
  }
  return channel
}