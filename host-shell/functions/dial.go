package driver

func (r *Root) DialImpl(opts *MountOpts) string {
	if client := r.OpenImpl(opts); client != nil {
		return client.URI
	}
	return ""
}
