import (
  "log"

  "github.com/heatxsink/go-hue/portal"
)

func RunDiscoveryImpl() *DiscoveredBridge {
  pp, err := portal.GetPortal()
  if err != nil {
    log.Println("Hue Bridge Discovery error:", err)
    return nil
  }

  // TODO: return a map once drivers can...
  // folder := inmem.NewFolder("discovered-bridges")
  for _, b := range pp {
    return &DiscoveredBridge{
      ID: b.ID,
      LanIPAddress: b.InternalIPAddress,
      MacAddress: b.MacAddress,
    }
  }

  // no bridge found
  return nil
}