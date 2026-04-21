package logmsg

// AutoParser tries JSON first, then logfmt, then falls back to passthrough.
// It is safe to use as a zero value.
type AutoParser struct {
	json   JSONParser
	logfmt LogfmtParser
}

func (p AutoParser) Parse(raw string) LogMessage {
	if m := p.json.Parse(raw); m.Format != "" {
		return m
	}
	if m := p.logfmt.Parse(raw); m.Format != "" {
		return m
	}
	return LogMessage{Raw: raw}
}

var _ Parser = AutoParser{}
