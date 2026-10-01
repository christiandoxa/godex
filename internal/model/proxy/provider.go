package proxy

type Provider struct {
	Kind             string
	Name             string
	Host             string
	Login            string
	APIURL           string
	DefaultModel     string
	ContextWindow    int64
	AutoCompactLimit int64
}
