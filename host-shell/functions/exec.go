import (
  "github.com/stardustapp/dustgo/lib/toolbox"
)

func (s *Session) ExecImpl(opts *ExecOpts) *Process {
  return &Process{
    Pid: "-1",
    Status: toolbox.NewReactiveString("status", "Pending"),
    ExitCode: toolbox.NewReactiveString("exit-code", ""),
    StdoutLatest: toolbox.NewReactiveString("stdout-latest", "-1"),
  };
}
