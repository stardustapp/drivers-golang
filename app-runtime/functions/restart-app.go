import "log"
import "strings"
import "time"
import "github.com/stardustapp/core/inmem"

func (a *App) RestartAppImpl() string {
  // mark app as stopping - prevents new stuff from starting
  // TODO: if the app is Stopped already, don't need to worry about stopping again.
  a.Status = "Stopping"
  log.Println("RestartApp: stopping", a.AppName)

  // list processes that are still running
  runningProcesses := make(map[string]*Process)
  for _, pId := range a.Processes.Children() {
    if processFolder, ok := a.Processes.Fetch(pId); ok {
      if p, ok := processFolder.(*Process); ok {

        if p.Status != "Completed" && !strings.HasPrefix(p.Status, "Terminated:") {
          runningProcesses[pId] = p
          p.AbortTime = time.Now().Format(time.RFC3339Nano)
        }
      }
    }
  }

  log.Println("RestartApp: Aborting", len(runningProcesses), "running processes")
  for {
    var stillRunning int
    for _, p := range runningProcesses {
      if p.Status != "Completed" && !strings.HasPrefix(p.Status, "Terminated:") {
        stillRunning += 1
      }
    }

    if stillRunning > 0 {
      log.Println("RestartApp: ", len(runningProcesses), "processes still running...")
    } else {
      log.Println("RestartApp: All processes have terminated.")
      break
    }

    time.Sleep(1000 * time.Millisecond)
  }

  // reset various state
  a.nextPid = 0
  a.Processes = inmem.NewFolder("processes")
  a.ctx.Put("state", inmem.NewFolder("state"))
  a.ctx.Put("export", inmem.NewFolder("export"))

  // fire it back up
  log.Println("RestartApp: State has been reset. Firing up the app again.")
  a.Status = "Ready"

  a.StartRoutineImpl(&ProcessParams{
    RoutineName: "launch",
  })

  return "ok";
}