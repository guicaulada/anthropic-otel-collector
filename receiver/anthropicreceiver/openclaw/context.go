package openclaw

import "net/http"

const (
	HeaderAgentID    = "X-OpenClaw-Agent-ID"
	HeaderSessionKey = "X-OpenClaw-Session-Key"
	HeaderChannel    = "X-OpenClaw-Channel"
	HeaderWorkspace  = "X-OpenClaw-Workspace"
	HeaderProvider   = "X-OpenClaw-Provider"
	HeaderTargetURL  = "X-OpenClaw-Target-URL"
)

type Context struct {
	AgentID    string
	SessionKey string
	Channel    string
	Workspace  string
	Provider   string
	TargetURL  string
}

func ExtractContext(req *http.Request) *Context {
	ctx := &Context{
		AgentID:    req.Header.Get(HeaderAgentID),
		SessionKey: req.Header.Get(HeaderSessionKey),
		Channel:    req.Header.Get(HeaderChannel),
		Workspace:  req.Header.Get(HeaderWorkspace),
		Provider:   req.Header.Get(HeaderProvider),
		TargetURL:  req.Header.Get(HeaderTargetURL),
	}
	return ctx
}

func (c *Context) IsOpenClaw() bool {
	return c.AgentID != "" || c.Channel != ""
}
