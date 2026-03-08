package dto

type ChatSessionListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	UpdatedAt    string `json:"updatedAt"`
}

type ChatSessionSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ChatMessageItem struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatSessionsResponse struct {
	Sessions []ChatSessionListItem `json:"sessions"`
}

type CreateChatSessionRequest struct {
	Title string `json:"title,omitempty"`
}

type CreateChatSessionResponse struct {
	Session ChatSessionListItem `json:"session"`
}

type ChatMessagesResponse struct {
	Session  ChatSessionSummary `json:"session"`
	Messages []ChatMessageItem  `json:"messages"`
}

type SendChatMessageRequest struct {
	Content string `json:"content"`
}

type SendChatMessageResponse struct {
	Session  ChatSessionSummary `json:"session"`
	Messages []ChatMessageItem  `json:"messages"`
}
