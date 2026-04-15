package chat

// Message is a single chat message in the OpenAI chat-completions
// format. Only role and content are used.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is the subset of the OpenAI chat-completions request body
// that logwiz sends.
type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// response mirrors the server response shape; unexported because
// callers only care about the extracted assistant content.
type response struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}
