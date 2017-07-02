import "github.com/jzelinskie/geddit"
import "strconv"

// Get reddit's default frontpage
func (s *Session) GetDefaultFrontpageImpl() *Submission {
  // Set listing options
  subOpts := geddit.ListingOptions{
    Limit: 1,
  }

  submissions, err := s.svc.DefaultFrontpage(geddit.DefaultPopularity, subOpts)
  if err != nil {
    panic("Error getting default frontpage: " + err.Error())
  }

  if len(submissions) > 0 {
    s := submissions[0]

    bannedBy := ""
    if s.BannedBy != nil {
      bannedBy = *s.BannedBy
    }
    bools := map[bool]string{
      true: "yes",
      false: "no",
    }

    return &Submission{
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

      DateCreated: strconv.FormatFloat(s.DateCreated, 'E', -1, 64),

      NumComments: strconv.Itoa(s.NumComments),
      Score: strconv.Itoa(s.Score),
      Ups: strconv.Itoa(s.Ups),
      Downs: strconv.Itoa(s.Downs),

      IsNsfw: bools[s.IsNSFW],
      IsSelf: bools[s.IsSelf],
      WasClicked: bools[s.WasClicked],
      IsSaved: bools[s.IsSaved],

      BannedBy: bannedBy,
    }
  }
  return nil
}