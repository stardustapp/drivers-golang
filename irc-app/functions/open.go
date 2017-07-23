import (
  "log"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
)

func OpenImpl(opts *AppOpts) *App {
  app := &App{
    N: inmem.NewFolder("networks"),
    Options: opts,
    State: "Pending",
  }

  go func(a *App) {
    for _, name := range opts.Servers.Children() {
      servEnt, ok := opts.Servers.Fetch(name)
      if !ok {
        continue
      }

      servFold := servEnt.(base.Folder)
      log.Println("Initializing server w/", servFold.Children())
    }

    app.State = "Ready"
  }(app)

  return app
}