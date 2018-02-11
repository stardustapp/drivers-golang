import (
  "log"
  "strconv"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
)

func (c *Client) GetTorrentsImpl() base.Folder {
  torrents, err := c.svc.GetTorrents()
  if err != nil {
    log.Println("deluge failed:", err)
    return nil
  }

  folder := inmem.NewFolder("torrents")
  idx := 0

  for _, t := range torrents {
    ent := &Torrent{
      Eta: strconv.FormatFloat(t.Eta, 'f', -1, 64),
      Hash: t.Hash,
      IsFinished: strconv.FormatBool(t.IsFinished),
      NumFiles: strconv.FormatFloat(t.NumFiles, 'f', -1, 64),
      NumPeers: strconv.FormatFloat(t.NumPeers, 'f', -1, 64),
      NumSeeds: strconv.FormatFloat(t.NumSeeds, 'f', -1, 64),
      Paused: strconv.FormatBool(t.Paused),
      Progress: strconv.FormatFloat(t.Progress, 'f', -1, 64),
      Queue: strconv.FormatFloat(t.Queue, 'f', -1, 64),
      Ratio: strconv.FormatFloat(t.Ratio, 'f', -1, 64),
      SavePath: t.SavePath,
      State: t.State,
      TorrentName: t.Name,
      TotalDone: strconv.FormatFloat(t.TotalDone, 'f', -1, 64),
      TotalPeers: strconv.FormatFloat(t.TotalPeers, 'f', -1, 64),
      TotalSeeds: strconv.FormatFloat(t.TotalSeeds, 'f', -1, 64),
      TotalSize: strconv.FormatFloat(t.TotalSize, 'f', -1, 64),
      TotalUploaded: strconv.FormatFloat(t.TotalUploaded, 'f', -1, 64),
      TrackerHost: t.TrackerHost,
    }
    folder.Put(strconv.Itoa(idx), ent)
    idx += 1
  }

  return folder
}