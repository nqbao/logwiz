package chat

// Message is a single chat message in the OpenAI chat-completions format.
// Only role and content are used.
type Message struct {
	Role    string
	Content string
}
