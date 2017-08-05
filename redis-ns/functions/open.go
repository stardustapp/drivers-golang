import (
  "log"

  "github.com/stardustapp/core/base"
  "github.com/stardustapp/core/extras"
  "github.com/stardustapp/core/inmem"

  "github.com/go-redis/redis"
)

// Performant Namesystem using Redis as a storage driver
// Kept seperate from the Redis API client, as this is very special-cased
// Assumes literally everything except credentials

func OpenImpl(opts *MountOpts) *Client {
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

  client := &Client{
    svc:    svc,
    prefix: opts.Prefix,
  }
  client.Root = client.getRoot()
  log.Printf("built client %+v", client)
  return client
}

func (c *Client) getRoot() base.Folder {
  rootNid := c.svc.Get(c.prefix + "root").Val()
  if rootNid == "" {
    log.Println("Initializing redisns root")
    rootNid = c.newNode("root", "Folder")
    c.svc.Set(c.prefix+"root", rootNid, 0)
  }
  return c.getEntry(rootNid).(base.Folder)
}

func (c *Client) newNode(name, typeStr string) string {
  nid := extras.GenerateId()
  if c.svc.SetNX(c.prefixFor(nid, "type"), typeStr, 0).Val() == false {
    panic("Redis node " + nid + " already exists, can't make new " + name + " " + typeStr)
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

func (c *Client) getEntry(nid string) base.Entry {
  name := c.nameOf(nid)
  prefix := c.prefixFor(nid, "")
  switch c.typeOf(nid) {

  case "String":
    value := c.svc.Get(prefix + "value").Val()
    return inmem.NewString(name, value)

  case "Link":
    value := c.svc.Get(prefix + "target").Val()
    return inmem.NewLink(name, value)

  case "File":
    // TODO: writable file struct!
    data, _ := c.svc.Get(prefix + "raw-data").Bytes()
    return inmem.NewFile(name, data)

  case "Folder":
    return &redisNsFolder{
      client: c,
      nid:    nid,
      prefix: prefix,
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

  entry = e.client.getEntry(nid)
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
    dest := e.client.getEntry(nid).(base.Folder)

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