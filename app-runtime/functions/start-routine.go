import (
  "time"
  "log"
  "strconv"
  "strings"

  "github.com/Shopify/go-lua"

  "github.com/stardustapp/core/skylink"
)

func (a *App) StartRoutineImpl(name string) *Process {
  pid := a.nextPid
  a.nextPid++

  p := &Process{
    App: a,
    RoutineName: name,
    ProcessID: strconv.Itoa(pid),
    StartedTime: time.Now().Format(time.RFC3339),
    Status: "Pending",
  }
  a.Processes.Put(p.ProcessID, p)

  go p.launch()
  return p
}

func (p *Process) launch() {
  log.Println("Starting routine", p)

  sourcePath := "/apps/" + p.App.AppName + "/routines/" + p.RoutineName + ".lua"
  source, ok := p.App.Session.ctx.GetFile(sourcePath)
  if !ok {
    p.Status = "Failed: file " + sourcePath + " not found"
    return
  }

  sourceText := string(source.Read(0, int(source.GetSize())))

  l := lua.NewState()
  //lua.OpenLibraries(l)

  _ = lua.NewMetaTable(l, "stardustContextMetaTable")
  lua.SetFunctions(l, []lua.RegistryFunction{
    {"startRoutine", func(l *lua.State) int {
      //k, v := lua.CheckString(l, 2), l.ToValue(3)
      //steps = append(steps, step{name: k, function: v})
      routineName := lua.CheckString(l, 1)
      log.Println("Lua started routine", routineName)
      p.App.StartRoutineImpl(routineName)
      // TODO: return routine's process
      return 0
    }},
    {"enumerate", func(l *lua.State) int {
      n := l.Top()
      log.Println("Lua got", n, "enumeration args")

      paths := make([]string, n)
      for i := range paths {
        log.Println("Lua enumeration idx:", i)
        paths[i] = lua.CheckString(l, i+1)
      }
      l.SetTop(0)

      path := "/"
      if n > 0 {
        path += strings.Join(paths, "/")
      } else {
        path = ""
      }
      log.Println("Lua enumeration on", path)

      startEntry, ok := p.App.Session.ctx.Get(path)
      if !ok {
        lua.Errorf(l, "Enumeration couldn't find path %s", path)
        panic("unreachable")
      }

      enum := skylink.NewEnumerator(p.App.Session.ctx, startEntry, 1)
      results := enum.Run() // <-chan nsEntry
      l.NewTable() // entry array
      idx := 0
      for res := range results {
        if idx > 0 {
          l.NewTable() // individual entry

          nameParts := strings.Split(res.Name, "/")
          baseName := nameParts[len(nameParts) - 1]

          l.PushString(baseName)
          l.SetField(2, "name")
          l.PushString(res.Name)
          l.SetField(2, "path")
          l.PushString(res.Type)
          l.SetField(2, "type")
          l.PushString(res.StringValue)
          l.SetField(2, "stringValue")

          l.RawSetInt(1, idx)
        }
        idx++
      }


      //k, v := lua.CheckString(l, 2), l.ToValue(3)
      //steps = append(steps, step{name: k, function: v})
      //routineName := lua.CheckString(l, 1)
      //log.Println("Lua started routine", routineName)
      //p.App.StartRoutineImpl(routineName)
      // TODO: return routine's process
      return 0
    }},
  }, 0)
  l.SetGlobal("ctx")

  p.Status = "Running"
  if err := lua.DoString(l, sourceText); err != nil {
    p.Status = "Failed: " + err.Error()
  } else {
    p.Status = "Completed"
  }
}