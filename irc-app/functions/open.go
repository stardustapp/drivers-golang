import (
  "log"
  "fmt"
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
      servFold := servEnt.(base.Folder)

      stateDir, ok := ctx.GetFolder(opts.State + "/" + name)
      if !ok {
        log.Println("Creating state directory for", name)
        stateDir = inmem.NewFolder(name)
        ctx.Put(opts.State + "/" + name, stateDir)
      }

      network := &Network{
        State: "Pending",
        N: inmem.NewFolder("contexts"),
        stateDir: stateDir,
        configDir: servEnt.(base.Folder),
      }
      app.N.Put(name, network)

      // Try for an existing connection
      if wireStr, ok := ctx.GetString(opts.State + "/" + name + "/wire-uri"); ok {
        if wire := openWire(wireStr.Get()); wire != nil {
          network.State = "Resuming existing connection"
          network.Wire = wire
          go network.runWire()
          continue
        } else {
          log.Println("Failed to resume wire", wireStr.Get())
        }
      }

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
        go network.runWire()
        continue
      }

      network.State = "Failed: Wire couldn't be grabbed"
    }

    app.State = "Ready"
  }(app)

  return app
}

// find or create context w/ log
func (n *Network) getContext(id string, ctxType string, label string) *Context {

  // use cache if possible
  if ent, ok := n.N.Fetch(id); ok {
    return ent.(*Context)
  }

  ctx := &Context{
    Type: ctxType,
    Label: label,
  }

  // Upsert server log folder
  logKey := "logs-" + id

  if logRootEnt, ok := n.stateDir.Fetch(logKey); ok { // TODO
    ctx.Log = logRootEnt.(base.Folder)
  } else {
    log.Println("Creating server-log root state directory")
    if ok := n.stateDir.Put(logKey, inmem.NewFolder(logKey)); !ok {
      n.State = "Failed: Couldn't create log " + logKey
      return nil
    }
    if logRootEnt, ok = n.stateDir.Fetch(logKey); ok {
      ctx.Log = logRootEnt.(base.Folder)
      ctx.Log.Put("horizon", inmem.NewString("id", ""))
      ctx.Log.Put("latest", inmem.NewString("id", ""))
    } else {
      n.State = "Failed: Couldn't get new log " + logKey
      return nil
    }
  }

  n.N.Put(id, ctx)
  log.Println("Assembled context", id)
  return ctx
}

func (c *Context) writeLog(when time.Time, msg string) {
  partition := when.UTC().Format("2006-01-02")
  timestamp := when.UTC().Format(time.RFC3339)

  if c.CurrentLog == nil || c.CurrentLog.Name() != partition {
    // Upsert this day's server log folder
    if logEnt, ok := c.Log.Fetch(partition); ok { // TODO
      c.CurrentLog = logEnt.(base.Folder)
    } else {
      log.Println("Creating", c.Label, "day state directory for", partition)
      if ok := c.Log.Put(partition, inmem.NewFolder(partition)); !ok {
        log.Println("Failed: Couldn't create log " + partition)
        return
      }
      if logEnt, ok = c.Log.Fetch(partition); ok { // TODO
        c.CurrentLog = logEnt.(base.Folder)
        c.CurrentLog.Put("horizon", inmem.NewString("id", "0"))
        c.CurrentLog.Put("latest", inmem.NewString("id", "-1"))
        c.Log.Put("latest", inmem.NewString("id", partition))
      } else {
        log.Println("Failed: Couldn't get new log " + partition)
        return
      }
    }
  }

  // Fetch next ID from stored state
  logNext := 0
  if logNextEnt, ok := c.CurrentLog.Fetch("latest"); ok {
    logNext, _ = strconv.Atoi(logNextEnt.(base.String).Get())
    logNext++
  }

  c.CurrentLog.Put(strconv.Itoa(logNext), inmem.NewString("log", timestamp + "|" + msg))
  c.CurrentLog.Put("latest", inmem.NewString("id", strconv.Itoa(logNext)))
  log.Println("Wrote message", logNext, "into", c.CurrentLog.Name(), "for", c.Label)
}

func (n *Network) runWire() {
  checkpoint := -1

  // Restore checkpoint from stored state
  if checkpointEnt, ok := n.stateDir.Fetch("wire-checkpoint"); ok {
    checkpoint, _ = strconv.Atoi(checkpointEnt.(base.String).Get())
  }
  log.Println("Resuming after checkpoint", checkpoint)

  // Cache wire's history folder
  historyEnt, _ := n.Wire.Fetch("history")
  historyFold := historyEnt.(base.Folder)

  sendEnt, _ := n.Wire.Fetch("send")
  sendFold, _ := sendEnt.(base.Folder)
  sendIvkEnt, _ := sendFold.Fetch("invoke")
  sendIvkFunc, _ := sendIvkEnt.(base.Function)
  sendMsg := func(command string, params ...string) {
    sendIvkFunc.Invoke(nil, inmem.NewFolderOf("msg",
      inmem.NewString("command", command),
      inmem.NewString("params", strings.Join(params, "|")),
    ))
  }

  serverCtx := n.getContext("server", "Server", "Server")

  for {
    // Check that we're still connected
    wireStateEnt, _ := n.Wire.Fetch("state")
    wireState := wireStateEnt.(base.String).Get()
    if wireState != "Pending" && wireState != "Ready" {
      serverCtx.writeLog(time.Now(), "Disconnected from server: " + wireState)
      n.State = "Failed: Wire was state " + wireState
      n.stateDir.Put("wire-uri", nil)
      break
    }

    // Check for any/all new content
    latestEnt, _ := n.Wire.Fetch("history-latest")
    latest, _ := strconv.Atoi(latestEnt.(base.String).Get())
    for checkpoint < latest {
      checkpoint += 1

      msgEnt, _ := historyFold.Fetch(strconv.Itoa(checkpoint))
      msgFold := msgEnt.(base.Folder)
      log.Println("New wire message:", msgFold)

      prefixNameStr, _ := msgFold.Fetch("prefix-name")
      prefixUserStr, _ := msgFold.Fetch("prefix-user")
      prefixHostStr, _ := msgFold.Fetch("prefix-host")
      commandStr, _ := msgFold.Fetch("command")
      paramsStr, _ := msgFold.Fetch("params")
      sourceStr, _ := msgFold.Fetch("source")
      timestampStr, _ := msgFold.Fetch("timestamp")

      timestamp, _ := time.Parse(time.RFC3339, timestampStr.(base.String).Get())
      params := strings.Split(paramsStr.(base.String).Get(), "|")

      msg := fmt.Sprintf("Received unhandled.. %v - %v - %v - %v - %v - %v",
                         sourceStr.(base.String).Get(),
                         commandStr.(base.String).Get(),
                         prefixNameStr.(base.String).Get(),
                         prefixUserStr.(base.String).Get(),
                         prefixHostStr.(base.String).Get(),
                         paramsStr.(base.String).Get())

      serverCtx.writeLog(timestamp, msg)

      switch commandStr.(base.String).Get() {
      case "001":
        channelEnt, _ := n.configDir.Fetch("channels")
        channelList := strings.Split(channelEnt.(base.String).Get(), ",")
        for _, channel := range channelList {
          sendMsg("JOIN", channel)
        }

      case "JOIN":
        ctx := n.getContext("chan-" + params[0], "Channel", params[0])
        ctx.writeLog(timestamp, "User joined: " + prefixNameStr.(base.String).Get())

      case "PART":
        ctx := n.getContext("chan-" + params[0], "Channel", params[0])
        ctx.writeLog(timestamp, "User left: " + prefixNameStr.(base.String).Get())

      case "PRIVMSG":
        ctx := n.getContext("chan-" + params[0], "Channel", params[0])
        ctx.writeLog(timestamp, "<" + prefixNameStr.(base.String).Get() + "> " + params[1])

      case "PING":

      default:
        serverCtx.writeLog(timestamp, msg)
      }

      n.stateDir.Put("wire-checkpoint", inmem.NewString("", strconv.Itoa(checkpoint)))
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