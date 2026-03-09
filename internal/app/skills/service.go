package skills

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/config"
)

const maxSkillZipSize = 25 << 20

type Service struct {
	configPath  string
	globalRoot  string
	builtinRoot string
}

func NewService() *Service {
	return &Service{
		configPath:  getConfigPath(),
		globalRoot:  globalSkillsPath(),
		builtinRoot: builtinSkillsPath(),
	}
}

func NewServiceWithPaths(configPath, globalRoot, builtinRoot string) *Service {
	return &Service{
		configPath:  configPath,
		globalRoot:  globalRoot,
		builtinRoot: builtinRoot,
	}
}

func (s *Service) ListSkills() (dto.SkillsResponse, error) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return dto.SkillsResponse{}, fmt.Errorf("load config: %w", err)
	}

	entries := make([]dto.SkillInventoryItem, 0)
	enabledBy := enabledSkillsByAgent(cfg)

	for _, root := range agentSkillRoots(cfg) {
		items, err := scanSkillsRoot(root.path, "agent-workspace", root.agentIDs, enabledBy)
		if err != nil {
			return dto.SkillsResponse{}, err
		}
		entries = append(entries, items...)
	}

	globalItems, err := scanSkillsRoot(s.globalRoot, "global", nil, enabledBy)
	if err != nil {
		return dto.SkillsResponse{}, err
	}
	entries = append(entries, globalItems...)

	builtinItems, err := scanSkillsRoot(s.builtinRoot, "builtin", nil, enabledBy)
	if err != nil {
		return dto.SkillsResponse{}, err
	}
	entries = append(entries, builtinItems...)

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Name < entries[j].Name
	})

	return dto.SkillsResponse{
		Skills:      entries,
		Communities: defaultCommunities(),
	}, nil
}

func (s *Service) GetSkillDetail(id string) (dto.SkillDetailResponse, error) {
	list, err := s.ListSkills()
	if err != nil {
		return dto.SkillDetailResponse{}, err
	}

	for _, skill := range list.Skills {
		if skill.ID != id {
			continue
		}

		content, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
		if err != nil {
			return dto.SkillDetailResponse{}, fmt.Errorf("read skill.md: %w", err)
		}

		return dto.SkillDetailResponse{
			Skill:   skill,
			Content: string(content),
		}, nil
	}

	return dto.SkillDetailResponse{}, fmt.Errorf("skill %q not found", id)
}

func (s *Service) DeleteSkill(id string) error {
	list, err := s.ListSkills()
	if err != nil {
		return err
	}

	for _, skill := range list.Skills {
		if skill.ID != id {
			continue
		}
		if skill.Source == "builtin" {
			return errors.New("builtin skills cannot be deleted")
		}
		if strings.TrimSpace(skill.Path) == "" {
			return errors.New("skill path is empty")
		}
		if err := os.RemoveAll(skill.Path); err != nil {
			return fmt.Errorf("delete skill: %w", err)
		}
		return nil
	}

	return fmt.Errorf("skill %q not found", id)
}

func (s *Service) UploadSkill(fileName string, file io.Reader) (dto.UploadSkillResponse, error) {
	if strings.TrimSpace(fileName) == "" {
		return dto.UploadSkillResponse{}, errors.New("file is required")
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".zip") {
		return dto.UploadSkillResponse{}, errors.New("file must be a zip archive")
	}
	if err := os.MkdirAll(s.globalRoot, 0o755); err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("create global skills root: %w", err)
	}

	tempFile, err := os.CreateTemp("", "picoclaw-skill-*.zip")
	if err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("create temp zip: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	written, err := io.Copy(tempFile, io.LimitReader(file, maxSkillZipSize+1))
	if err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("store zip: %w", err)
	}
	if written > maxSkillZipSize {
		return dto.UploadSkillResponse{}, errors.New("zip file is too large")
	}
	if err := tempFile.Close(); err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("finalize zip: %w", err)
	}

	reader, err := zip.OpenReader(tempPath)
	if err != nil {
		return dto.UploadSkillResponse{}, errors.New("invalid zip archive")
	}
	defer func() { _ = reader.Close() }()

	layout, err := detectSkillArchiveLayout(fileName, reader.File)
	if err != nil {
		return dto.UploadSkillResponse{}, err
	}
	skillName := layout.skillName

	targetDir := filepath.Join(s.globalRoot, skillName)
	if _, err := os.Stat(targetDir); err == nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("skill %q already exists", skillName)
	}

	tempDir, err := os.MkdirTemp("", "picoclaw-skill-unzip-*")
	if err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	destDir := filepath.Join(tempDir, skillName)
	if err := extractSkillArchive(reader.File, layout, destDir); err != nil {
		return dto.UploadSkillResponse{}, err
	}
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		return dto.UploadSkillResponse{}, errors.New("skill archive missing SKILL.md")
	}
	if err := os.Rename(destDir, targetDir); err != nil {
		return dto.UploadSkillResponse{}, fmt.Errorf("move skill into global root: %w", err)
	}

	return dto.UploadSkillResponse{
		Skill: dto.SkillInventoryItem{
			ID:          inventoryID("global", skillName, targetDir),
			Name:        skillName,
			Description: summarizeSkillMarkdown(filepath.Join(targetDir, "SKILL.md")),
			Source:      "global",
			Path:        targetDir,
		},
	}, nil
}

type skillRoot struct {
	path     string
	agentIDs []string
}

func agentSkillRoots(cfg *config.Config) []skillRoot {
	roots := make(map[string][]string)

	mainWorkspace := resolveAgentWorkspace(nil, &cfg.Agents.Defaults)
	roots[filepath.Join(mainWorkspace, "skills")] = []string{"main"}

	for i := range cfg.Agents.List {
		agentCfg := &cfg.Agents.List[i]
		agentID := normalizedAgentID(agentCfg)
		workspace := resolveAgentWorkspace(agentCfg, &cfg.Agents.Defaults)
		root := filepath.Join(workspace, "skills")
		roots[root] = appendUnique(roots[root], agentID)
	}

	items := make([]skillRoot, 0, len(roots))
	for path, agentIDs := range roots {
		items = append(items, skillRoot{
			path:     path,
			agentIDs: agentIDs,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].path < items[j].path
	})

	return items
}

func scanSkillsRoot(rootPath, source string, ownerAgents []string, enabledBy map[string][]string) ([]dto.SkillInventoryItem, error) {
	if strings.TrimSpace(rootPath) == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills root %s: %w", rootPath, err)
	}

	items := make([]dto.SkillInventoryItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(rootPath, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}

		name := entry.Name()
		items = append(items, dto.SkillInventoryItem{
			ID:          inventoryID(source, name, skillDir),
			Name:        name,
			Description: summarizeSkillMarkdown(skillFile),
			Source:      source,
			Path:        skillDir,
			OwnerAgents: append([]string(nil), ownerAgents...),
			EnabledBy:   append([]string(nil), enabledBy[name]...),
		})
	}

	return items, nil
}

func enabledSkillsByAgent(cfg *config.Config) map[string][]string {
	result := make(map[string][]string)
	for i := range cfg.Agents.List {
		agentCfg := &cfg.Agents.List[i]
		agentID := normalizedAgentID(agentCfg)
		for _, name := range agentCfg.Skills {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			result[trimmed] = appendUnique(result[trimmed], agentID)
		}
	}
	return result
}

func inventoryID(source, name, path string) string {
	return fmt.Sprintf("%s:%s:%s", source, name, path)
}

func normalizedAgentID(agentCfg *config.AgentConfig) string {
	if agentCfg == nil || strings.TrimSpace(agentCfg.ID) == "" {
		return "main"
	}
	id := strings.TrimSpace(agentCfg.ID)
	if id == "main" {
		return "main"
	}
	return id
}

func resolveAgentWorkspace(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) string {
	if agentCfg != nil && strings.TrimSpace(agentCfg.Workspace) != "" {
		return expandHome(strings.TrimSpace(agentCfg.Workspace))
	}
	if agentCfg == nil || agentCfg.Default || strings.TrimSpace(agentCfg.ID) == "" || normalizedAgentID(agentCfg) == "main" {
		return expandHome(defaults.Workspace)
	}
	return filepath.Join(expandHome(defaults.Workspace), "..", "workspace-"+normalizedAgentID(agentCfg))
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}

func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

func getGlobalConfigDir() string {
	if home := os.Getenv("PICOCLAW_HOME"); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picoclaw")
}

func globalSkillsPath() string {
	return filepath.Join(getGlobalConfigDir(), "skills")
}

func builtinSkillsPath() string {
	if path := strings.TrimSpace(os.Getenv("PICOCLAW_BUILTIN_SKILLS")); path != "" {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(wd, "skills"),
		filepath.Join(wd, "workspace", "skills"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func defaultCommunities() []dto.SkillCommunityItem {
	return []dto.SkillCommunityItem{
		{ID: "clawhub", Name: "ClawHub", URL: "https://clawhub.ai/"},
		{ID: "github", Name: "GitHub", URL: "https://github.com/topics/agent-skills"},
	}
}

func summarizeSkillMarkdown(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if description := extractDescriptionFromFrontmatter(text); description != "" {
		return description
	}

	return extractFirstParagraph(stripFrontmatter(text))
}

func extractDescriptionFromFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}

	lines := strings.Split(content, "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if !strings.HasPrefix(line, "description:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		value = strings.Trim(value, "\"'")
		return value
	}
	return ""
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}

	lines := strings.Split(content, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}

func extractFirstParagraph(content string) string {
	lines := strings.Split(content, "\n")
	parts := make([]string, 0, 3)
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}
		if trimmed == "" {
			if len(parts) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "|") ||
			strings.HasPrefix(trimmed, "- ") ||
			strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "name:") ||
			strings.HasPrefix(trimmed, "description:") ||
			strings.HasPrefix(trimmed, "homepage:") ||
			strings.HasPrefix(trimmed, "metadata:") {
			continue
		}

		parts = append(parts, trimmed)
	}

	if len(parts) == 0 {
		return ""
	}

	paragraph := strings.Join(parts, " ")
	runes := []rune(paragraph)
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return paragraph
}

type skillArchiveLayout struct {
	skillName  string
	rootPrefix string
}

func detectSkillArchiveLayout(fileName string, files []*zip.File) (skillArchiveLayout, error) {
	roots := make(map[string]struct{})
	rootFiles := make(map[string]struct{})
	var nestedSkillName string

	for _, file := range files {
		cleanName := filepath.Clean(file.Name)
		if cleanName == "." || strings.HasPrefix(cleanName, "..") {
			return skillArchiveLayout{}, errors.New("invalid zip archive path")
		}
		if cleanName == "__MACOSX" || strings.HasPrefix(cleanName, "__MACOSX"+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(cleanName, string(filepath.Separator))
		if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
			roots[parts[0]] = struct{}{}
		}
		if filepath.Base(cleanName) == "SKILL.md" {
			if len(parts) == 1 {
				rootFiles["SKILL.md"] = struct{}{}
				continue
			}
			nestedSkillName = parts[0]
		}
	}

	if len(rootFiles) > 0 {
		if _, exists := rootFiles["SKILL.md"]; exists {
			return skillArchiveLayout{
				skillName:  inferRootlessSkillName(fileName),
				rootPrefix: "",
			}, nil
		}
	}

	if nestedSkillName == "" {
		return skillArchiveLayout{}, errors.New("zip 中未找到合法的 SKILL.md")
	}
	if len(roots) != 1 {
		return skillArchiveLayout{}, errors.New("zip must contain a single top-level skill directory")
	}
	return skillArchiveLayout{
		skillName:  nestedSkillName,
		rootPrefix: nestedSkillName + string(filepath.Separator),
	}, nil
}

func inferRootlessSkillName(fileName string) string {
	base := strings.TrimSpace(filepath.Base(fileName))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.TrimSpace(base)
	if base == "" {
		return "skill"
	}
	return base
}

func extractSkillArchive(files []*zip.File, layout skillArchiveLayout, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	for _, file := range files {
		cleanName := filepath.Clean(file.Name)
		if cleanName == "__MACOSX" || strings.HasPrefix(cleanName, "__MACOSX"+string(filepath.Separator)) {
			continue
		}
		if layout.rootPrefix != "" && cleanName == layout.skillName {
			continue
		}

		relativePath := cleanName
		if layout.rootPrefix != "" {
			if !strings.HasPrefix(cleanName, layout.rootPrefix) {
				return errors.New("zip contains files outside the skill root directory")
			}
			relativePath = strings.TrimPrefix(cleanName, layout.rootPrefix)
		}
		if relativePath == "" {
			continue
		}
		if strings.HasPrefix(relativePath, "..") {
			return errors.New("zip contains invalid relative paths")
		}

		targetPath := filepath.Join(destDir, relativePath)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(filepath.Separator)) &&
			filepath.Clean(targetPath) != filepath.Clean(destDir) {
			return errors.New("zip contains unsafe paths")
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create dir: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}

		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("open zip file entry: %w", err)
		}

		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("create extracted file: %w", err)
		}

		if _, err := io.Copy(dst, src); err != nil {
			_ = src.Close()
			_ = dst.Close()
			return fmt.Errorf("extract file: %w", err)
		}

		_ = src.Close()
		_ = dst.Close()
	}

	return nil
}
