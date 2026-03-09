package dto

type SkillInventoryItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Source      string   `json:"source"`
	Path        string   `json:"path"`
	OwnerAgents []string `json:"ownerAgents,omitempty"`
	EnabledBy   []string `json:"enabledBy,omitempty"`
}

type SkillCommunityItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type SkillsResponse struct {
	Skills      []SkillInventoryItem `json:"skills"`
	Communities []SkillCommunityItem `json:"communities"`
}

type UploadSkillResponse struct {
	Skill SkillInventoryItem `json:"skill"`
}

type SkillDetailResponse struct {
	Skill   SkillInventoryItem `json:"skill"`
	Content string             `json:"content"`
}
