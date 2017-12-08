import (
  "log"
  "errors"
  "os"
  "fmt"
  "strings"
  "net"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/extras"
  "github.com/stardustapp/core/inmem"
  "github.com/stardustapp/core/skylink"

  "github.com/go-redis/redis"
)

// Performant subscribable namesystem using Redis as a storage driver
// Kept seperate from the Redis API client, as this is very special-cased
// Assumes literally everything except credentials

func (r *Root) OpenImpl(opts *MountOpts) *Client {
  if opts.Address == "" {
    opts.Address = "localhost:6379"
  }
  if opts.Prefix == "" {
    opts.Prefix = "sdns:"
  }

  svc := redis.NewClient(&redis.Options{
    Addr:     opts.Address,
    Password: opts.Password,
    DB:       0, // use default DB
  })


  // TODO: this should be made already
  if r.Sessions == nil {
    r.Sessions = inmem.NewFolder("sessions")
  }
  sessionId := extras.GenerateId()

  // Return absolute URI to the created session
  name, err := os.Hostname()
  if err != nil {
    log.Println("Oops 1:", err)
    return nil // "Err! no ip"
  }
  addrs, err := net.LookupHost(name)
  if err != nil {
    log.Println("Oops 2:", err)
    return nil // "Err! no host"
  }
  if len(addrs) < 1 {
    log.Println("Oops 2:", err)
    return nil // "Err! no host ip"
  }
  selfIp := addrs[0]
  uri := fmt.Sprintf("skylink+ws://%s:9234/pub/sessions/%s", selfIp, sessionId)

  client := &Client{
    svc:    svc,
    prefix: opts.Prefix,
    URI:    uri,
  }
  client.Root = client.getRoot()
  log.Printf("built client %+v", client)

  if ok := r.Sessions.Put(sessionId, client); !ok {
    log.Println("Session store rejected us :(", err)
    return nil
  }
  return client
}

func (c *Client) getRoot() base.Folder {
  rootNid := c.svc.Get(c.prefix + "root").Val()
  if rootNid == "" {
    log.Println("Initializing redisns root")
    rootNid = c.newNode("root", "Folder")
    c.svc.Set(c.prefix+"root", rootNid, 0)
  }
  return c.getEntry(rootNid, false).(base.Folder)
}

func (c *Client) newNode(name, typeStr string) string {
  nid := extras.GenerateId()
  if c.svc.SetNX(c.prefixFor(nid, "type"), typeStr, 0).Val() == false {
    // TODO: try another time probably, don't just crash
    log.Println("WARN: Redis node", nid, "already exists, couldn't make new", name, typeStr, "- retrying")
    nid = extras.GenerateId()
    if c.svc.SetNX(c.prefixFor(nid, "type"), typeStr, 0).Val() == false {
      log.Fatalln("Redis node", nid, "already exists, can't make new", name, typeStr)
    }
  }

  c.svc.Set(c.prefixFor(nid, "name"), name, 0)
  log.Println("Created redisns node", nid, "named", name, "type", typeStr)
  return nid
}

func (c *Client) prefixFor(nid, key string) string {
  return c.prefix + "nodes/" + nid + ":" + key
}

func (c *Client) nameOf(nid string) string {
  return c.svc.Get(c.prefixFor(nid, "name")).Val()
}
func (c *Client) typeOf(nid string) string {
  return c.svc.Get(c.prefixFor(nid, "type")).Val()
}

func (c *Client) getEntry(nid string, shallow bool) base.Entry {
  name := c.nameOf(nid)
  prefix := c.prefixFor(nid, "")
  switch c.typeOf(nid) {

  case "String":
    value := c.svc.Get(prefix + "value").Val()
    str := inmem.NewString(name, value)
    if shallow {
      return str
    } else {
      return &redisNsString{
        client: c,
        nid: nid,
        String: str,
      }
    }

  case "Link":
    value := c.svc.Get(prefix + "target").Val()
    return inmem.NewLink(name, value)

  case "File":
    // TODO: writable file struct!
    data, _ := c.svc.Get(prefix + "raw-data").Bytes()
    return inmem.NewFile(name, data)

  case "Folder":
    if shallow {
      return inmem.NewFolder(name)
    } else {
      return &redisNsFolder{
        client: c,
        nid:    nid,
        prefix: prefix,
      }
    }

  default:
    log.Println("redisns key", nid, name, "has unknown type", c.typeOf(nid))
    return nil
  }
}

// Persists as a Folder from an redisNs instance
// Presents as a dynamic name tree
type redisNsFolder struct {
  client *Client
  prefix string
  nid    string
}

var _ base.Folder = (*redisNsFolder)(nil)

func (e *redisNsFolder) Name() string {
  return e.client.nameOf(e.nid)
}

func (e *redisNsFolder) Children() []string {
  names, _ := e.client.svc.HKeys(e.prefix + "children").Result()
  return names
}

func (e *redisNsFolder) Fetch(name string) (entry base.Entry, ok bool) {
  nid := e.client.svc.HGet(e.prefix+"children", name).Val()
  if nid == "" {
    return nil, false
  }

  entry = e.client.getEntry(nid, false)
  if str, ok := entry.(*redisNsString); ok {
    str.parent = e
    str.field = name
  }
  ok = entry != nil
  return
}

// replaces whatever node reference was already there w/ a new node
func (e *redisNsFolder) Put(name string, entry base.Entry) (ok bool) {
  if entry == nil {
    // unlink a child, leaves it around tho
    // TODO: garbage collection!
    e.client.svc.HDel(e.prefix+"children", name)
    return true
  }

  var nid string
  switch entry := entry.(type) {

  case *redisNsFolder:
    // the folder already exists in redis, make a reference
    nid = entry.nid

  case base.Folder:
    nid = e.client.newNode(entry.Name(), "Folder")
    dest := e.client.getEntry(nid, false).(base.Folder)

    // recursively copy entire folder to redis
    for _, child := range entry.Children() {
      if childEnt, ok := entry.Fetch(child); ok {
        dest.Put(child, childEnt)
      } else {
        log.Println("redisns: Failed to get child", child, "of", name)
      }
    }

  case base.String:
    nid = e.client.newNode(entry.Name(), "String")
    e.client.svc.Set(e.client.prefixFor(nid, "value"), entry.Get(), 0)

  case base.Link:
    nid = e.client.newNode(entry.Name(), "Link")
    e.client.svc.Set(e.client.prefixFor(nid, "target"), entry.Target(), 0)

  case base.File:
    nid = e.client.newNode(entry.Name(), "File")

    size := entry.GetSize()
    data := entry.Read(0, int(size))
    e.client.svc.Set(e.client.prefixFor(nid, "raw-data"), data, 0)

  }

  if nid == "" {
    log.Println("redisns put failed for", name, "on node", e.nid)
    return false
  } else {
    e.client.svc.HSet(e.prefix+"children", name, nid)
    return true
  }
}

type redisNsString struct {
  client *Client
  nid    string // nid of initial value
  parent *redisNsFolder // ref to parent's nid
  field  string // parent's name for child
  *inmem.String // has child's own name
}

var _ base.String = (*redisNsString)(nil)


///////////////////////////////////////////
// Experimental subscribe() impl
// Here be dragons!

type subState struct {
  client *Client
  sub      *skylink.Subscription

  nidMap map[string]*subNode
}

type subNode struct {
  nid      string
  children map[string]*subNode
  path     string
  height   int // remaining children depths
}

func (n *subNode) load(state *subState, asChanged bool) {
  // send self
  entry := state.client.getEntry(n.nid, true)
  if asChanged {
    state.sub.SendNotification("Changed", n.path, entry)
  } else {
    state.sub.SendNotification("Added", n.path, entry)
  }

  // recurse into any children
  if n.height > 0 {
    prefix := n.path
    if prefix != "" {
      prefix += "/"
    }

    childKey := state.client.prefixFor(n.nid, "children")
    for name, nid := range state.client.svc.HGetAll(childKey).Val() {
      node := &subNode{
        nid: nid,
        children: make(map[string]*subNode),
        path: prefix+name,
        height: n.height - 1,
      }
      n.children[name] = node
      log.Println("adding redis node", nid, "path", n.path, "to sub nidMap")
      state.nidMap[nid] = node
      node.load(state, false)
    }
  }
}

func (n *subNode) unload(state *subState, andRemove bool) {
  log.Println("unloading redis node", n.nid, "path", n.path)

  // remove children first
  if n.children != nil {
    for _, child := range n.children {
      child.unload(state, true)
    }
  }

  delete(state.nidMap, n.nid)
  if andRemove {
    state.sub.SendNotification("Removed", n.path, nil)
  }
}

func (n *subNode) processEvent(action, field string, state *subState) {
  //log.Println("redis node", n.nid, "path", n.path, "received", action, "event on", field)

  if (field == "children") {
    // ignore if not recursive
    if n.height <= 0 {
      log.Println("redis node", n.nid, "path", n.path, "ignoring child event - not recursive")
      return
    }

    prefix := n.path
    if prefix != "" {
      prefix += "/"
    }

    childKey := state.client.prefixFor(n.nid, "children")
    //changed := make(map[string]string) // name => new-nid
    seen := make(map[string]bool)
    for name, nid := range state.client.svc.HGetAll(childKey).Val() {
      seen[name] = true

      var alreadyExisted bool
      // check if child name already existed
      if node, ok := n.children[name]; ok {
        if node.nid == nid {
          // child reference didn't change
          continue
        }

        // child changed nid, remove the old node
        node.unload(state, false)
        alreadyExisted = true
        delete(n.children, name)
        log.Println("update: child", name, "changed nid to", nid, "from", node.nid)
      }

      // add the new child
      node := &subNode{
        nid: nid,
        children: make(map[string]*subNode),
        path: prefix+name,
        height: n.height - 1,
      }
      n.children[name] = node
      log.Println("update: adding nid", nid, "path", n.path, "to sub nidMap")
      state.nidMap[nid] = node
      node.load(state, alreadyExisted)
    }

    // find old names that weren't mentioned
    for name, node := range n.children {
      if seen[name] {
        continue
      }

      // remove the deleted node
      node.unload(state, false)
      delete(n.children, name)
      log.Println("update: child", name, "nid", node.nid, "was removed")
    }

  } else {
    log.Println("WARN: redis node", n.nid, "path", n.path, "got unimpl event", action, field)
  }
}

func (e *redisNsFolder) Subscribe(s *skylink.Subscription) (err error) {
  if resp := e.client.svc.ConfigSet("notify-keyspace-events", "AK"); resp.Err() != nil {
    log.Println("Couldn't configure keyspace events.", resp.Err())
  }
  log.Println("Starting redis-ns sub")

  pubsub := e.client.svc.Subscribe()
  pattern := "__keyspace@0__:"+e.client.prefix+"nodes/*"
  if err := pubsub.PSubscribe(pattern); err != nil {
    return errors.New("redis sub error: "+err.Error())
  }

  // build up map of nodes we initially see / care about
  state := &subState{
    client: e.client,
    sub: s,
    nidMap: make(map[string]*subNode),
  }

  go func(stopC <-chan struct{}) {
    log.Println("setting folder sub pubsub closer")
    <-stopC
    log.Println("closing folder sub pubsub")
    pubsub.Close()
  }(s.StopC)

  go func(state *subState) {
    defer log.Println("stopped sub loop")
    defer s.Close()

    rootNode := &subNode{
      nid: e.nid,
      children: make(map[string]*subNode),
      path: "",
      height: s.MaxDepth,
    }
    state.nidMap[e.nid] = rootNode
    rootNode.load(state, false)
    s.SendNotification("Ready", "", nil)

    log.Println("starting sub loop")
    for msg := range pubsub.Channel() {
      msgKey := msg.Channel[(len(pattern)-1):]
      parts := strings.Split(msgKey, ":")
      msgNid := parts[0]
      msgField := parts[1]

      if node, ok := state.nidMap[msgNid]; ok {
        node.processEvent(msg.Payload, msgField, state)
      }
    }
  }(state)

  return nil // errors.New("not implemented yet")
}





///////////////////////////////////////////
// Experimental subscribe() impl for single strings
// Here be dragons!

type strSubState struct {
  client *Client
  sub      *skylink.Subscription

  childNid string
  parentNid string
  parentField string
}

/*
  entry := state.client.getEntry(n.nid, true)
  state.sub.SendNotification("Added", n.path, entry)

  state.sub.SendNotification("Removed", n.path, nil)
*/

func (e *redisNsString) Subscribe(s *skylink.Subscription) (err error) {
  if resp := e.client.svc.ConfigSet("notify-keyspace-events", "AK"); resp.Err() != nil {
    log.Println("Couldn't configure keyspace events.", resp.Err())
  }
  log.Println("Starting redis-ns string sub")

  pubsub := e.client.svc.Subscribe()

  childKey := e.client.prefixFor(e.parent.nid, "children")
  evtKey := "__keyspace@0__:"+childKey
  if err := pubsub.Subscribe(evtKey); err != nil {
    return errors.New("redis string sub error: "+err.Error())
  }

  //parentNid: e.parent.nid,
  //parentField: e.field
  //childNid: e.nid

  go func(stopC <-chan struct{}) {
    log.Println("setting string sub pubsub closer")
    <-stopC
    log.Println("closing string sub pubsub")
    pubsub.Close()
  }(s.StopC)

  go func() {
    defer log.Println("stopped string sub loop")
    defer s.Close()

    latestNid := e.client.svc.HGet(childKey, e.field).Val()
    if latestNid != "" {
      s.SendNotification("Added", "", e.client.getEntry(latestNid, true))
    }
    s.SendNotification("Ready", "", nil)

    log.Println("starting string sub loop")
    for msg := range pubsub.Channel() {
      log.Println("string sub received payload", msg.Payload, "for", e.parent.nid)
      newNid := e.client.svc.HGet(childKey, e.field).Val()
      if newNid == latestNid {
        continue
      } else if newNid == "" {
        s.SendNotification("Removed", "", nil)
      } else if latestNid == "" {
        s.SendNotification("Added", "", e.client.getEntry(newNid, true))
      } else {
        s.SendNotification("Changed", "", e.client.getEntry(newNid, true))
      }
      log.Println("string sub nid changed from", latestNid, "to", newNid)
      latestNid = newNid
    }
  }()

  return nil // errors.New("not implemented yet")
}
