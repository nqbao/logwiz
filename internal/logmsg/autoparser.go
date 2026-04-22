package logmsg

// AutoParser tries JSON first, then logfmt, then falls back to passthrough.
// It is safe to use as a zero value.
type AutoParser struct {
	json   JSONParser
	logfmt LogfmtParser
}

func (p AutoParser) Parse(raw string) []LogMessage {
	if ms := p.json.Parse(raw); ms[0].Format != "" {
		return ms
	}
	if ms := p.logfmt.Parse(raw); ms[0].Format != "" {
		return ms
	}
	return []LogMessage{{Raw: raw}}
}

func (AutoParser) Flush() []LogMessage { return nil }

var _ Parser = AutoParser{}
