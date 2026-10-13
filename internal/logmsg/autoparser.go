package logmsg

// AutoParser tries JSON first, then logfmt, then pipe-delimited (with
// stack-trace continuation lines), then falls back to passthrough.
// It is safe to use as a zero value but must be used via a pointer.
type AutoParser struct {
	json   JSONParser
	logfmt LogfmtParser
	pipe   PipeParser
}

func (p *AutoParser) Parse(raw string) []LogMessage {
	if ms := p.json.Parse(raw); ms[0].Format != "" {
		p.pipe.reset()
		return ms
	}
	if ms := p.logfmt.Parse(raw); ms[0].Format != "" {
		p.pipe.reset()
		return ms
	}
	return p.pipe.Parse(raw)
}

func (*AutoParser) Flush() []LogMessage { return nil }

var _ Parser = (*AutoParser)(nil)
