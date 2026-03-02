package openclaw

func AttributeMap(ctx *Context) map[string]string {
	if ctx == nil {
		return nil
	}
	attrs := make(map[string]string)
	if ctx.AgentID != "" {
		attrs["openclaw.agent.id"] = ctx.AgentID
	}
	if ctx.SessionKey != "" {
		attrs["openclaw.session.key"] = ctx.SessionKey
	}
	if ctx.Channel != "" {
		attrs["openclaw.channel"] = ctx.Channel
	}
	if ctx.Workspace != "" {
		attrs["openclaw.workspace"] = ctx.Workspace
	}
	if ctx.Provider != "" {
		attrs["openclaw.provider"] = ctx.Provider
	}
	return attrs
}
