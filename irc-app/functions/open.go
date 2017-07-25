import (
  "log"
  "time"
  "strconv"
  "strings"
  "net/url"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/skylink"
)

func OpenImpl(opts *AppOpts) *App {
  app := &App{
    N: inmem.NewFolder("networks"),
    Options: opts,
    State: "Pending",
  }

  go func(a *App) {
    log.Println("Starting up!", opts.ChartURI)

    chart := openSkylink(opts.ChartURI)
    if chart == nil {
      app.State = "Failed: Chart not available: " + opts.ChartURI
      return
    }
    ns := base.NewNamespace(opts.ChartURI, chart)
    ctx := base.NewRootContext(ns)

    serverCfgs, ok := ctx.GetFolder(opts.Servers)
    if !ok {
      app.State = "Failed: Server configs not found"
      return
    }

    stateRoot, ok := ctx.GetFolder(opts.State)
    if !ok {
      log.Println("Creating state root directory")
      stateRoot = inmem.NewFolder("irc-app-state")
      ctx.Put(opts.State, stateRoot)
    }

    for _, name := range serverCfgs.Children() {
      servEnt, ok := serverCfgs.Fetch(name)
      if !ok {
        continue
      }

      stateDir, ok := ctx.GetFolder(opts.State + "/" + name)
      if !ok {
        log.Println("Creating state directory for", name)
        stateDir = inmem.NewFolder(name)
        ctx.Put(opts.State + "/" + name, stateDir)
      }

      network := &Network{
        State: "Pending",
      }
      app.N.Put(name, network)

      // Try for an existing connection
      if wireStr, ok := ctx.GetString(opts.State + "/" + name + "/wire-uri"); ok {
        if wire := openWire(wireStr.Get()); wire != nil {
          network.State = "Resuming existing connection"
          network.Wire = wire
          go network.runWire(stateDir)
          continue
        }
      }

      servFold := servEnt.(base.Folder)
      if len(servFold.Children()) < 2 {
        continue
      }
      log.Println("Initializing server", name, "w/", servFold.Children())

      dialFunc, ok := ctx.GetFunction(opts.Dialer + "/invoke")
      if !ok {
        network.State = "Failed: No dialer function provided"
        continue
      }

      network.State = "Dialing..."
      sessStr := dialFunc.Invoke(nil, servEnt)
      if sessStr == nil {
        network.State = "Failed: Dialer didn't work :("
        continue
      }
      sessUri := sessStr.(base.String).Get()
      stateDir.Put("wire-uri", sessStr)
      stateDir.Put("wire-checkpoint", inmem.NewString("", "-1"))

      if wire := openWire(sessUri); wire != nil {
        network.State = "Dialed @ " + sessUri
        network.Wire = wire
        go network.runWire(stateDir)
        continue
      }

      network.State = "Failed: Wire couldn't be grabbed"
    }

    app.State = "Ready"
  }(app)

  return app
}

func (n *Network) runWire(stateDir base.Folder) {
  checkpoint := -1

  // Restore checkpoint from stored state
  if checkpointEnt, ok := stateDir.Fetch("wire-checkpoint"); ok {
    checkpoint, _ = strconv.Atoi(checkpointEnt.(base.String).Get())
  }
  log.Println("Resuming after checkpoint", checkpoint)

  // Cache wire's history folder
  historyEnt, _ := n.Wire.Fetch("history")
  historyFold := historyEnt.(base.Folder)

  for {
    // Check that we're still connected
    wireStateEnt, _ := n.Wire.Fetch("state")
    wireState := wireStateEnt.(base.String).Get()
    if wireState != "Pending" && wireState != "Ready" {
      n.State = "Failed: Wire was state " + wireState
      stateDir.Put("wire-uri", nil)
      break
    }

    // Check for any/all new content
    latestEnt, _ := n.Wire.Fetch("history-latest")
    latest, _ := strconv.Atoi(latestEnt.(base.String).Get())
    for checkpoint < latest {
      checkpoint += 1

      msgEnt, _ := historyFold.Fetch(strconv.Itoa(checkpoint))
      msgFold := msgEnt.(base.Folder)
      log.Println("Inbound message:", msgFold)

      stateDir.Put("wire-checkpoint", inmem.NewString("", strconv.Itoa(checkpoint)))
    }

    // Sleep a sec
    time.Sleep(time.Second)
  }
}

func openWire(wireURI string) base.Folder {
  uri, err := url.Parse(wireURI)
  if err != nil {
    log.Println("Skylink Wire URI parsing failed.", wireURI, err)
    return nil
  }

  skylink := openSkylink(uri.Scheme + "://" + uri.Host)
  if skylink == nil {
    return nil
  }
  skyNs := base.NewNamespace("tmp:/", skylink)
  skyCtx := base.NewRootContext(skyNs)

  subPath, _ := skyCtx.GetFolder(uri.Path)
  return subPath
}

func openSkylink(linkURI string) base.Entry {
  uri, err := url.Parse(linkURI)
  if err != nil {
    log.Println("Skylink URI parsing failed.", linkURI, err)
    return nil
  }

  log.Println("Importing", uri.Scheme, uri.Host)
  switch uri.Scheme {

  case "skylink+http", "skylink+https":
    actualUri := strings.TrimPrefix(linkURI, "skylink+") + "/~~export"
    return skylink.ImportUri(actualUri)

  case "skylink+ws", "skylink+wss":
    actualUri := strings.TrimPrefix(linkURI, "skylink+") + "/~~export/ws"
    return skylink.ImportUri(actualUri)

  case "skylink":
    names := strings.Split(uri.Host, ".")
    if len(names) == 3 && names[2] == "local" && names[1] == "chart" {

      // import the cluster-local skychart endpoint
      skychart := openSkylink("skylink+ws://skychart")
      skyNs := base.NewNamespace("skylink://skychart.local", skychart)
      skyCtx := base.NewRootContext(skyNs)

      invokeFunc, ok := skyCtx.GetFunction("/pub/open/invoke")
      if !ok {
        log.Println("Skychart didn't offer open function.")
        return nil
      }

      chartMeta := invokeFunc.Invoke(nil, inmem.NewString("", names[0]))
      if chartMeta == nil {
        log.Println("Skychart couldn't open", names[0], "chart")
        return nil
      }

      chartMetaF := chartMeta.(base.Folder)
      browseE, _ := chartMetaF.Fetch("browse")
      browseF := browseE.(base.Folder)
      invokeE, _ := browseF.Fetch("invoke")
      browseFunc := invokeE.(base.Function)

      chart := browseFunc.Invoke(nil, nil)
      if chart == nil {
        log.Println("Skychart couldn't browse", names[0], "chart")
        return nil
      }

      return chart

    } else {
      log.Println("Unknown skylink URI hostname", uri.Host)
      return nil
    }

  default:
    log.Println("Unknown skylink URI scheme", uri.Scheme)
    return nil
  }
}