import (
  "log"
  "os"
  "fmt"
  "strings"
  "io/ioutil"
  "path/filepath"

  "github.com/stardustapp/dustgo/lib/base"
  "github.com/stardustapp/dustgo/lib/extras"
  "github.com/stardustapp/dustgo/lib/inmem"
  "github.com/stardustapp/dustgo/lib/toolbox"

  //"github.com/erikdubbelboer/gspt"
)

// Presents a faithful representation of the underlying host filesystem
// Exposes a simple File/Folder-only namesystem
// Also allows arbitrary POSIX process execution

func (r *Root) OpenImpl(opts *MountOpts) *Session {

  //gspt.SetProcTitle("sky-anchor")

  // Return absolute URI to the created session
  if r.Sessions == nil {
    // TODO: this should be made already
    r.Sessions = inmem.NewObscuredFolder("sessions")
  }
  sessionId := extras.GenerateId()
  sessionPath := fmt.Sprintf(":9234/pub/sessions/%s", sessionId)
  sessionUri, _ := toolbox.SelfURI(sessionPath)

  session := &Session{
    fsPrefix: filepath.Clean(opts.FsRootPath),
    URI:      sessionUri,
  }
  session.FsRoot = session.getRoot()
  log.Printf("built session %+v", session)

  if ok := r.Sessions.Put(sessionId, session); !ok {
    log.Println("Session store rejected us :(")
    return nil
  }
  return session
}

func (s *Session) getRoot() base.Folder {
  return &hostFolder{
    name: "fs-root",
    session: s,
    prefix: s.fsPrefix,
  }
}


// Persists as a Folder from the host OS
// Presents as a dynamic name tree
type hostFolder struct {
  name string
  session *Session
  prefix string
}

var _ base.Folder = (*hostFolder)(nil)

func (e *hostFolder) Name() string {
  return e.name
}

func (e *hostFolder) Children() []string {
  files, err := ioutil.ReadDir(e.prefix)
  if err != nil {
    log.Println("WARN Children():", err)
    return []string{"error"}
  }

  names := make([]string, 0, len(files))
  for _, file := range files {
    names = append(names, file.Name())
  }
  return names
}

func (e *hostFolder) Fetch(name string) (entry base.Entry, ok bool) {
  if strings.ContainsAny(name, "\r\n\x00/") {
    log.Printf("WARN Fetch(): child name %q has restricted chars", name)
    return nil, false
  }

  subPath := filepath.Clean(filepath.Join(e.prefix, name))
  if !strings.HasPrefix(subPath, e.prefix) {
    log.Println("WARN Fetch(): subpath", subPath, "isn't in prefix", e.prefix)
    return nil, false
  }

  // figure out what the path refers to
  fileInfo, err := os.Stat(subPath)
  if err != nil {
    log.Println("WARN Fetch(): stat", subPath, "failed:", err)
    return nil, false
  }

  // we support files and folders only, try 'em
  switch {
  case fileInfo.Mode().IsDir():
    return &hostFolder{
      name: name,
      session: e.session,
      prefix: subPath,
    }, true

  case fileInfo.Mode().IsRegular():
    data, err := ioutil.ReadFile(subPath)
    if err != nil {
      log.Println("WARN Fetch() ReadFile(", subPath, ":", err)
      return nil, false
    }

    return inmem.NewFile(name, data), true

  default:
    log.Println("WARN Fetch():", subPath, "has weird mode:", fileInfo.Mode())
  }
  return nil, false
}

// create/replace/remove a child with a new inode
func (e *hostFolder) Put(name string, entry base.Entry) (ok bool) {
  if strings.ContainsAny(name, "\r\n\x00/") {
    log.Printf("WARN Put(): child name %q has restricted chars", name)
    return false
  }

  subPath := filepath.Clean(filepath.Join(e.prefix, name))
  if !strings.HasPrefix(subPath, e.prefix) {
    log.Println("WARN Put(): subpath", subPath, "isn't in prefix", e.prefix)
    return false
  }

  // figure out what the path refers to
  fileInfo, statErr := os.Lstat(subPath)
  fileExists := true
  if statErr != nil {
    if os.IsNotExist(statErr) {
      fileExists = false
    } else {
      log.Println("WARN Put(): stat", subPath, "failed:", statErr)
      return false
    }
  }

  // handle deletions
  if entry == nil {
    if fileExists {
      log.Println("WARN Put(): DELETING", subPath)
      if err := os.Remove(subPath); err != nil {
        log.Println("WARN Put(): FAILED to delete", subPath, err)
        return false
      }
    }
    return true
  }

  // figure out what we got
  switch entry := entry.(type) {

  // TODO: this is basically a hardlink. do we want to support them?
  //case *hostFolder:

  case base.Folder:
    // don't worry about contents (unlike redis)
    // just try making it
    err := os.Mkdir(subPath, 0755)
    if err != nil {
      log.Println("WARN Put(): FAILED to mkdir", subPath, err)
      return false
    }
    log.Println("WARN Put(): SUCCESSFULLY MKDIRD", subPath)

  case base.File:
    if statErr == nil && !fileInfo.Mode().IsRegular() {
      log.Println("WARN Put(): tried to write", subPath, "but is already non-file")
      return false
    }

    size := entry.GetSize()
    data := entry.Read(0, int(size))

    // write it
    err := ioutil.WriteFile(subPath, data, 0644)
    if err != nil {
      log.Println("WARN Put(): FAILED to write", len(data), "bytes to", subPath, err)
      return false
    }
    log.Println("WARN Put(): SUCCESSFULLY WROTE", len(data), "bytes to", subPath)

  default:
    log.Println("WARN Put(): unsupported entry for", subPath)
    return false
  }

  return true
}
