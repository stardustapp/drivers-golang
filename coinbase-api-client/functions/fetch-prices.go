import "log"
import "net/http"
import "time"
import "encoding/json"
import "github.com/stardustapp/dustgo/lib/inmem"

var client = &http.Client{Timeout: 5 * time.Second}

func FetchPricesImpl() *PriceInfo {
  log.Println("Making coinbase request")

  r, err := client.Get("https://api.coinbase.com/v2/prices/USD/spot")
  if err != nil {
    log.Println("coinbase HTTP error:", err)
    return &PriceInfo{}
  }
  defer r.Body.Close()

  var resp apiResp
  err = json.NewDecoder(r.Body).Decode(&resp)
  if err != nil {
    log.Println("coinbase JSON decoder error:", err)
    return &PriceInfo{}
  }

  info := &PriceInfo{
    Currency: resp.Data[0].Currency,
    Prices: inmem.NewFolder("prices"),
  }
  for _, line := range resp.Data {
    log.Println("Coinbase offered", line.Amount, "for 1", line.Base)
    info.Prices.Put(line.Base, inmem.NewString("price", line.Amount))
  }
  return info
}

type apiResp struct {
  Data []priceEntry
}
type priceEntry struct {
  Base string
  Currency string
  Amount string
}
