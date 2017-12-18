func (s *Session) ExecImpl(opts *ExecOpts) *Process {
  return &Process{
    Pid: "-1",
    Status: "Pending",
  };
}