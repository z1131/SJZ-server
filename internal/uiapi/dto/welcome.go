package dto

// BootstrapResponse represents the initial data for the welcome page
type BootstrapResponse struct {
	CurrentStep  string                `json:"currentStep"`
	ModelOptions []ModelOption         `json:"modelOptions"`
	QQ           QQBootstrapConfig     `json:"qq"`
}

// ModelOption represents a model choice for the welcome page
type ModelOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Provider    string `json:"provider"`
	AuthMode    string `json:"authMode"`
	Recommended bool   `json:"recommended,omitempty"`
	Description string `json:"description"`
}

// QQBootstrapConfig represents QQ channel configuration for the welcome page
type QQBootstrapConfig struct {
	GuideURL string           `json:"guideUrl"`
	Fields   []QQConfigField  `json:"fields"`
}

// QQConfigField represents a single field in QQ configuration
type QQConfigField struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	Placeholder  string `json:"placeholder,omitempty"`
}

// SaveModelRequest represents a request to save model setup
type SaveModelRequest struct {
	ModelID  string `json:"modelId"`
	Provider string `json:"provider"`
	AuthMode string `json:"authMode"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
}

// SaveModelResponse represents the response after saving model setup
type SaveModelResponse struct {
	NextStep string `json:"nextStep"`
}

// SaveQQRequest represents a request to save QQ channel configuration
type SaveQQRequest struct {
	AppID     string `json:"appId"`
	AppSecret string `json:"appSecret"`
}

// SaveQQResponse represents the response after saving QQ configuration
type SaveQQResponse struct {
	Completed bool `json:"completed"`
}
