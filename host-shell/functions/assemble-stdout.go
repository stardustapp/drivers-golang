import (
  "github.com/stardustapp/dustgo/lib/base"
  "github.com/stardustapp/dustgo/lib/inmem"
)

func (p *Process) AssembleStdoutImpl() base.File {
  return inmem.NewFile("stdout", []byte{})
}
