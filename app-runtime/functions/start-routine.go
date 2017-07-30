import (
  "time"
  "log"
  "fmt"
  "strconv"
  "strings"

  "github.com/Shopify/go-lua"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/skylink"
  "github.com/stardustapp/core/toolbox"
)

func (a *App) StartRoutineImpl(name string) *Process {
  pid := a.nextPid
  a.nextPid++

  p := &Process{
    App: a,
    RoutineName: name,
    ProcessID: strconv.Itoa(pid),
    StartTime: time.Now().Format(time.RFC3339),
    Status: "Pending",
  }
  a.Processes.Put(p.ProcessID, p)

  go p.launch()
  return p
}

func (p *Process) launch() {
  log.Println("Starting routine", p)

  sourcePath := "/source/routines/" + p.RoutineName + ".lua"
  source, ok := p.App.ctx.GetFile(sourcePath)
  if !ok {
    p.Status = "Failed: file " + sourcePath + " not found"
    return
  }

  sourceText := string(source.Read(0, int(source.GetSize())))

  l := lua.NewState()
  lua.OpenLibraries(l)

  _ = lua.NewMetaTable(l, "stardust/base.Context")
  l.Pop(1)

  // Reads all the lua arguments and resolves a context for them
  resolveLuaPath := func (l *lua.State) (ctx base.Context, path string) {
    // Discover the context at play
    if userCtx := lua.TestUserData(l, 1, "stardust/base.Context"); userCtx != nil {
      ctx = userCtx.(base.Context)
      l.Remove(1)
    } else {
      ctx = p.App.ctx
    }

    // Read in the path strings
    n := l.Top()
    paths := make([]string, n)
    for i := range paths {
      paths[i] = lua.CheckString(l, i+1)
    }
    l.SetTop(0)

    // Create a path
    path = "/"
    if n > 0 {
      path += strings.Join(paths, "/")
    } else {
      path = ""
    }
    return
  }

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

    // TODO: add readonly 'chroot' variant, returns 'nil' if not exist
    {"mkdirp", func(l *lua.State) int {
      ctx, path := resolveLuaPath(l)
      log.Println("Lua mkdirp to", path, "from", ctx.Name())

      if ok := toolbox.Mkdirp(ctx, path); !ok {
        lua.Errorf(l, "mkdirp() couldn't create folders for path %s", path)
        panic("unreachable")
      }

      subRoot, ok := ctx.GetFolder(path)
      if !ok {
        lua.Errorf(l, "mkdirp() couldn't find folder at path %s", path)
        panic("unreachable")
      }
      subNs := base.NewNamespace(ctx.Name() + path, subRoot)
      subCtx := base.NewRootContext(subNs)

      l.PushUserData(subCtx)
      lua.MetaTableNamed(l, "stardust/base.Context")
      l.SetMetaTable(-2)
      return 1
    }},

    {"read", func(l *lua.State) int {
      ctx, path := resolveLuaPath(l)
      log.Println("Lua read from", path, "from", ctx.Name())

      if str, ok := ctx.GetString(path); ok {
        l.PushString(str.Get())
      } else {
        log.Println("lua read() failed to find string at path", path)
        l.PushString("")
      }
      return 1
    }},

    {"enumerate", func(l *lua.State) int {
      ctx, path := resolveLuaPath(l)
      log.Println("Lua enumeration on", path, "from", ctx.Name())

      startEntry, ok := ctx.Get(path)
      if !ok {
        lua.Errorf(l, "enumeration() couldn't find path %s", path)
        panic("unreachable")
      }

      enum := skylink.NewEnumerator(p.App.ctx, startEntry, 1)
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
      return 1
    }},

    {"log", func(l *lua.State) int {
      n := l.Top()
      parts := make([]string, n)
      for i := range parts {
        switch l.TypeOf(i+1) {
        case lua.TypeString:
          parts[i] = lua.CheckString(l, i+1)
        case lua.TypeNumber:
          parts[i] = fmt.Sprintf("%v", lua.CheckNumber(l, i+1))
        default:
          parts[i] = fmt.Sprintf("[lua %s]", l.TypeOf(i+1).String())
        }
      }
      l.SetTop(0)

      log.Println("Lua log:", strings.Join(parts, " "))
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
  p.EndTime = time.Now().Format(time.RFC3339)
}