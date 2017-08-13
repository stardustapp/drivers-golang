import pushjet "github.com/geir54/goPushJet"
import "log"
import "time"

func CreateServiceImpl(cfg *ServiceConfig) *Service {
  svc, err := pushjet.CreateService(cfg.SvcName, cfg.Icon)
  if err != nil {
    log.Println("Failed to create PushJet service:", err)
    return nil
  }

  return &Service{
    CreatedAt: time.Unix(int64(svc.Created), 0).Format(time.RFC3339),
    Icon: svc.Icon,
    SvcName: svc.Name,
    Public: svc.Public,
    Secret: svc.Secret,
  }
}