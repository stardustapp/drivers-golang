package driver

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Shopify/go-lua"

	"github.com/stardustapp/dustgo/lib/base"
	"github.com/stardustapp/dustgo/lib/extras"
	"github.com/stardustapp/dustgo/lib/inmem"
	"github.com/stardustapp/dustgo/lib/skylink"
	"github.com/stardustapp/dustgo/lib/toolbox"
)

func (a *App) StartRoutineImpl(params *ProcessParams) *Process {
	// Don't create more processes when something is shutting down
	// TODO: maybe allow a cleanup process to clear some persisted state
	if a.Status != "Ready" && a.Status != "Pending" {
		log.Println("Refusing to start process", params,
			"as app", a.AppName, "is currently", a.Status)
		return nil
	}

	pid := a.nextPid
	a.nextPid++

	p := &Process{
		App:       a,
		Params:    params,
		ProcessID: strconv.Itoa(pid),
		StartTime: time.Now().Format(time.RFC3339Nano),
		Status:    "Pending",
	}
	a.Processes.Put(p.ProcessID, p)

	go p.launch()
	return p
}

func readLuaEntry(l *lua.State, index int) base.Entry {
	switch l.TypeOf(index) {

	case lua.TypeNil:
		return nil

	case lua.TypeString:
		str := lua.CheckString(l, index)
		return inmem.NewString("string", str)

	case lua.TypeNumber:
		str := fmt.Sprintf("%v", lua.CheckNumber(l, index))
		return inmem.NewString("number", str)

	case lua.TypeBoolean:
		if l.ToBoolean(index) {
			return inmem.NewString("boolean", "yes")
		} else {
			return inmem.NewString("boolean", "no")
		}

	case lua.TypeUserData:
		// base.Context values are passed back by-ref
		// TODO: can have a bunch of other interesting userdatas
		userCtx := lua.CheckUserData(l, index, "stardust/base.Context")
		ctx := userCtx.(base.Context)
		log.Println("Lua passed native star-context", ctx.Name())
		entry, _ := ctx.Get(".")
		return entry

	case lua.TypeTable:
		// Tables become folders
		l.PushValue(index)
		folder := inmem.NewFolder("input")
		l.PushNil() // Add nil entry on stack (need 2 free slots).
		for l.Next(-2) {
			key, _ := l.ToString(-2)
			val := readLuaEntry(l, -1)
			l.Pop(1) // Remove val, but need key for the next iter.
			folder.Put(key, val)
		}
		l.Pop(1)
		return folder

	default:
		lua.Errorf(l, "Stardust received unmanagable thing of type %s", l.TypeOf(index).String())
		panic("unreachable")
	}
}

func pushLuaTable(l *lua.State, folder base.Folder) {
	l.NewTable()
	for _, key := range folder.Children() {
		child, _ := folder.Fetch(key)
		switch child := child.(type) {
		case nil:
			l.PushNil()
		case base.String:
			l.PushString(child.Get())
		case base.Folder:
			pushLuaTable(l, child)
		default:
			lua.Errorf(l, "Directory entry %s in %s wasn't a recognizable type %s", key, folder.Name(), reflect.TypeOf(child))
			panic("unreachable")
		}
		l.SetField(-2, key)
	}
}

// Reads all the lua arguments and resolves a context for them
func resolveLuaPath(l *lua.State, parentCtx base.Context) (ctx base.Context, path string) {
	// Discover the context at play
	if userCtx := lua.TestUserData(l, 1, "stardust/base.Context"); userCtx != nil {
		ctx = userCtx.(base.Context)
		l.Remove(1)
	} else {
		ctx = parentCtx
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

func (p *Process) launch() {
	metaLog := "lua process [chart:" + p.App.Session.ChartURL + " app:" + p.App.AppName + " routine:" + p.Params.RoutineName + " pid:" + p.ProcessID + "]"
	log.Println(metaLog, "Starting routine")

	sourcePath := "/source/routines/" + p.Params.RoutineName + ".lua"
	source, ok := p.App.ctx.GetFile(sourcePath)
	if !ok {
		p.Status = "Failed: file " + sourcePath + " not found"
		return
	}

	sourceText := string(source.Read(0, int(source.GetSize())))

	l := lua.NewState()
	lua.OpenLibraries(l)

	// Type marker for native base.Context objects
	_ = lua.NewMetaTable(l, "stardust/base.Context")
	l.Pop(1)

	// If we have input, make up a table and expose it as global
	if p.Params.Input != nil {
		pushLuaTable(l, p.Params.Input)
		l.SetGlobal("input")
	}

	checkProcessHealth := func(l *lua.State) {
		if p.AbortTime != "" {
			log.Println(metaLog, "received abort signal")
			p.Status = "Aborted"
			lua.Errorf(l, "Stardust process received abort signal at %s", p.AbortTime)
		}
	}

	_ = lua.NewMetaTable(l, "stardustContextMetaTable")
	lua.SetFunctions(l, []lua.RegistryFunction{

		// ctx.startRoutine(name[, inputTable])
		{"startRoutine", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:startRoutine", "app:"+p.App.AppName)

			//k, v := lua.CheckString(l, 2), l.ToValue(3)
			//steps = append(steps, step{name: k, function: v})
			params := &ProcessParams{
				ParentID:    p.ProcessID,
				RoutineName: lua.CheckString(l, 1),
			}

			if l.Top() == 2 && l.IsTable(2) {
				log.Println(metaLog, "Reading Lua table for routine input", params.RoutineName)
				params.Input = readLuaEntry(l, 2).(base.Folder)
			}

			log.Printf(metaLog, "started routine %+v", params)
			p.App.StartRoutineImpl(params)
			// TODO: return routine's process
			return 0
		}},

		// ctx.mkdirp([pathRoot,] pathParts string...) Context
		// TODO: add readonly 'chroot' variant, returns 'nil' if not exist
		{"mkdirp", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:mkdirp", "app:"+p.App.AppName)

			ctx, path := resolveLuaPath(l, p.App.ctx)
			log.Println(metaLog, "mkdirp to", path, "from", ctx.Name())

			if ok := toolbox.Mkdirp(ctx, path); !ok {
				lua.Errorf(l, "mkdirp() couldn't create folders for path %s", path)
				panic("unreachable")
			}

			subRoot, ok := ctx.GetFolder(path)
			if !ok {
				lua.Errorf(l, "mkdirp() couldn't find folder at path %s", path)
				panic("unreachable")
			}
			subNs := base.NewNamespace(ctx.Name()+path, subRoot)
			subCtx := base.NewRootContext(subNs)

			l.PushUserData(subCtx)
			lua.MetaTableNamed(l, "stardust/base.Context")
			l.SetMetaTable(-2)
			return 1
		}},

		// ctx.import(wireUri) Context
		{"import", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:import", "app:"+p.App.AppName)

			wireUri := lua.CheckString(l, 1)
			log.Println(metaLog, "opening wire", wireUri)
			p.Status = "Waiting: Dialing " + wireUri

			// TODO: support abort interruptions
			if wire, ok := openWire(wireUri); ok {
				log.Println(metaLog, "Lua successfully opened wire", wireUri)

				// create a new base.Context
				subNs := base.NewNamespace(wireUri, wire)
				subCtx := base.NewRootContext(subNs)

				// return a Lua version of the ctx
				l.PushUserData(subCtx)
				lua.MetaTableNamed(l, "stardust/base.Context")
				l.SetMetaTable(-2)

			} else {
				log.Println(metaLog, "failed to open wire", wireUri)
				l.PushNil()
			}

			checkProcessHealth(l)
			p.Status = "Running"
			return 1
		}},

		// ctx.read([pathRoot,] pathParts string...) (val string)
		{"read", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:read", "app:"+p.App.AppName)

			ctx, path := resolveLuaPath(l, p.App.ctx)
			log.Println(metaLog, "read from", path, "from", ctx.Name())

			if str, ok := ctx.GetString(path); ok {
				l.PushString(str.Get())
			} else {
				log.Println(metaLog, "read() failed to find string at path", path)
				l.PushString("")
			}
			return 1
		}},

		// ctx.readDir([pathRoot,] pathParts string...) (val table)
		// TODO: reimplement as an enumeration
		{"readDir", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:readDir", "app:"+p.App.AppName)

			ctx, path := resolveLuaPath(l, p.App.ctx)
			log.Println(metaLog, "readdir on", path, "from", ctx.Name())

			if folder, ok := ctx.GetFolder(path); ok {
				pushLuaTable(l, folder)
			} else {
				l.NewTable()
				log.Println(metaLog, "readdir() failed to find folder at path", path)
			}
			return 1
		}},

		// ctx.store([pathRoot,] pathParts string..., thingToStore any) (ok bool)
		{"store", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:store", "app:"+p.App.AppName)

			// get the thing to store off the end
			entry := readLuaEntry(l, -1)
			l.Pop(1)
			// read all remaining args as a path
			ctx, path := resolveLuaPath(l, p.App.ctx)

			// make sure we're not unlinking
			if entry == nil {
				lua.Errorf(l, "store() can't store nils, use ctx.unlink()")
				panic("unreachable")
			}

			// do the thing
			log.Println(metaLog, "store to", path, "from", ctx.Name(), "of", entry)
			l.PushBoolean(ctx.Put(path, entry))
			return 1
		}},

		// ctx.invoke([pathRoot,] pathParts string..., input any) (output any)
		{"invoke", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:invoke", "app:"+p.App.AppName)

			// get the thing to store off the end, can be nil
			input := readLuaEntry(l, -1)
			l.Pop(1)

			// read all remaining args as a path
			ctx, path := resolveLuaPath(l, p.App.ctx)
			p.Status = "Blocked: Invoking " + ctx.Name() + path + " since " + time.Now().Format(time.RFC3339Nano)
			log.Println(metaLog, "invoke of", path, "from", ctx.Name(), "with input", input)

			ivk, ok := ctx.GetFunction(path + "/invoke")
			if !ok {
				lua.Errorf(l, "Tried to invoke function %s%s but did not exist", ctx.Name(), path)
				panic("unreachable")
			}

			output := ivk.Invoke(p.App.ctx, input)
			checkProcessHealth(l)

			// try returning useful results
			switch output := output.(type) {

			case base.String:
				l.PushString(output.Get())

			default:
				// unknown = just return a context to it
				subNs := base.NewNamespace("output:/", output)
				subCtx := base.NewRootContext(subNs)

				l.PushUserData(subCtx)
				lua.MetaTableNamed(l, "stardust/base.Context")
				l.SetMetaTable(-2)
			}

			p.Status = "Running"
			return 1
		}},

		// ctx.unlink([pathRoot,] pathParts string...) (ok bool)
		{"unlink", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:unlink", "app:"+p.App.AppName)

			ctx, path := resolveLuaPath(l, p.App.ctx)
			log.Println(metaLog, "unlink of", path, "from", ctx.Name())

			// do the thing
			l.PushBoolean(ctx.Put(path, nil))
			return 1
		}},

		// ctx.enumerate([pathRoot,] pathParts string...) []Entry
		// Entry tables have: name, path, type, stringValue
		{"enumerate", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:enumerate", "app:"+p.App.AppName)

			ctx, path := resolveLuaPath(l, p.App.ctx)
			log.Println(metaLog, "enumeration on", path, "from", ctx.Name())

			startEntry, ok := ctx.Get(path)
			if !ok {
				lua.Errorf(l, "enumeration() couldn't find path %s", path)
				panic("unreachable")
			}

			enum := skylink.NewEnumerator(p.App.ctx, startEntry, 1)
			results := enum.Run() // <-chan nsEntry
			l.NewTable()          // entry array
			idx := 0
			for res := range results {
				if idx > 0 {
					l.NewTable() // individual entry

					nameParts := strings.Split(res.Name, "/")
					baseName := nameParts[len(nameParts)-1]

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

		// ctx.log(messageParts string...)
		{"log", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:log", "app:"+p.App.AppName)

			n := l.Top()
			parts := make([]string, n)
			for i := range parts {
				switch l.TypeOf(i + 1) {
				case lua.TypeString:
					parts[i] = lua.CheckString(l, i+1)
				case lua.TypeNumber:
					parts[i] = fmt.Sprintf("%v", lua.CheckNumber(l, i+1))
				case lua.TypeUserData:
					userCtx := lua.CheckUserData(l, i+1, "stardust/base.Context")
					parts[i] = userCtx.(base.Context).Name()

				default:
					parts[i] = fmt.Sprintf("[lua %s]", l.TypeOf(i+1).String())
				}
			}
			l.SetTop(0)

			log.Println(metaLog, "debug log:", strings.Join(parts, " "))
			return 0
		}},

		// ctx.sleep(milliseconds int)
		{"sleep", func(l *lua.State) int {
			checkProcessHealth(l)
			extras.MetricIncr("runtime.syscall", "call:sleep", "app:"+p.App.AppName)
			// TODO: support interupting to abort

			ms := lua.CheckInteger(l, 1)
			p.Status = "Sleeping: Since " + time.Now().Format(time.RFC3339Nano)
			time.Sleep(time.Duration(ms) * time.Millisecond)

			checkProcessHealth(l)
			p.Status = "Running"
			return 0
		}},

		// ctx.timestamp() string
		{"timestamp", func(l *lua.State) int {
			extras.MetricIncr("runtime.syscall", "call:timestamp", "app:"+p.App.AppName)
			l.PushString(time.Now().UTC().Format(time.RFC3339))
			return 1
		}},

		// ctx.splitString(fulldata string, knife string) []string
		{"splitString", func(l *lua.State) int {
			extras.MetricIncr("runtime.syscall", "call:splitString", "app:"+p.App.AppName)
			str := lua.CheckString(l, 1)
			knife := lua.CheckString(l, 2)
			l.SetTop(0)

			l.NewTable()
			for idx, part := range strings.Split(str, knife) {
				l.PushString(part)
				l.RawSetInt(1, idx+1)
			}
			return 1
		}},
	}, 0)
	l.SetGlobal("ctx")

	p.Status = "Running"
	if err := lua.DoString(l, sourceText); err != nil {
		p.Status = "Terminated: " + err.Error()
	} else {
		p.Status = "Completed"
	}
	log.Println(metaLog, "stopped:", p.Status)
	p.EndTime = time.Now().Format(time.RFC3339Nano)
}
