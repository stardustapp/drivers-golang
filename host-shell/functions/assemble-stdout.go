import (
  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/inmem"
)

func (p *Process) AssembleStdoutImpl() base.File {
  return inmem.NewFile("stdout", []byte{})
}