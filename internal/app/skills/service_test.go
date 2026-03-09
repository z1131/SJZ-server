package skills

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestListSkillsScansRealRoots(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	globalRoot := filepath.Join(tmpDir, "global-skills")
	builtinRoot := filepath.Join(tmpDir, "builtin-skills")
	mainWorkspace := filepath.Join(tmpDir, "workspace")
	sideWorkspace := filepath.Join(tmpDir, "workspace-side")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = mainWorkspace
	cfg.Agents.List = []config.AgentConfig{
		{
			ID:        "side",
			Name:      "side",
			Workspace: sideWorkspace,
			Skills:    []string{"weather"},
		},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	mustWriteSkill(t, filepath.Join(mainWorkspace, "skills", "main-only"), "main skill")
	mustWriteSkill(t, filepath.Join(sideWorkspace, "skills", "weather"), "weather skill")
	mustWriteSkill(t, filepath.Join(globalRoot, "global-tool"), "global skill")
	mustWriteSkill(t, filepath.Join(builtinRoot, "builtin-tool"), "builtin skill")

	service := NewServiceWithPaths(configPath, globalRoot, builtinRoot)
	resp, err := service.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}

	if len(resp.Skills) != 4 {
		t.Fatalf("expected 4 skills, got %d", len(resp.Skills))
	}

	var foundWeather bool
	for _, item := range resp.Skills {
		if item.Name != "weather" {
			continue
		}
		foundWeather = true
		if item.Source != "agent-workspace" {
			t.Fatalf("expected weather source agent-workspace, got %s", item.Source)
		}
		if len(item.OwnerAgents) != 1 || item.OwnerAgents[0] != "side" {
			t.Fatalf("unexpected owner agents: %#v", item.OwnerAgents)
		}
		if len(item.EnabledBy) != 1 || item.EnabledBy[0] != "side" {
			t.Fatalf("unexpected enabledBy: %#v", item.EnabledBy)
		}
	}

	if !foundWeather {
		t.Fatal("weather skill not found")
	}
}

func TestUploadSkillStoresIntoGlobalRoot(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	globalRoot := filepath.Join(tmpDir, "global-skills")
	builtinRoot := filepath.Join(tmpDir, "builtin-skills")

	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	service := NewServiceWithPaths(configPath, globalRoot, builtinRoot)
	payload := buildSkillZip(t, "weather")

	resp, err := service.UploadSkill("weather.zip", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("UploadSkill failed: %v", err)
	}

	if resp.Skill.Source != "global" {
		t.Fatalf("expected source global, got %s", resp.Skill.Source)
	}

	if _, err := os.Stat(filepath.Join(globalRoot, "weather", "SKILL.md")); err != nil {
		t.Fatalf("uploaded skill not found in global root: %v", err)
	}
}

func TestUploadSkillSupportsRootLevelZip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	globalRoot := filepath.Join(tmpDir, "global-skills")
	builtinRoot := filepath.Join(tmpDir, "builtin-skills")

	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	service := NewServiceWithPaths(configPath, globalRoot, builtinRoot)
	payload := buildRootLevelSkillZip(t)

	resp, err := service.UploadSkill("desktop-control-1.0.0.zip", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("UploadSkill failed: %v", err)
	}

	if resp.Skill.Name != "desktop-control-1.0.0" {
		t.Fatalf("unexpected skill name: %s", resp.Skill.Name)
	}

	if _, err := os.Stat(filepath.Join(globalRoot, "desktop-control-1.0.0", "SKILL.md")); err != nil {
		t.Fatalf("uploaded root-level skill not found in global root: %v", err)
	}
}

func mustWriteSkill(t *testing.T, dir string, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := []byte("# skill\n\n" + description + "\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func buildSkillZip(t *testing.T, name string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)

	skillFile, err := zipWriter.Create(filepath.Join(name, "SKILL.md"))
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := skillFile.Write([]byte("# skill\n\nuploaded skill\n")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return buffer.Bytes()
}

func buildRootLevelSkillZip(t *testing.T) []byte {
	t.Helper()

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)

	skillFile, err := zipWriter.Create("SKILL.md")
	if err != nil {
		t.Fatalf("create root SKILL.md: %v", err)
	}
	if _, err := skillFile.Write([]byte("---\nname: desktop-control\ndescription: root zip\n---\n")); err != nil {
		t.Fatalf("write root SKILL.md: %v", err)
	}

	metaFile, err := zipWriter.Create("_meta.json")
	if err != nil {
		t.Fatalf("create _meta.json: %v", err)
	}
	if _, err := metaFile.Write([]byte("{}")); err != nil {
		t.Fatalf("write _meta.json: %v", err)
	}

	macosxFile, err := zipWriter.Create("__MACOSX/._SKILL.md")
	if err != nil {
		t.Fatalf("create __MACOSX entry: %v", err)
	}
	if _, err := macosxFile.Write([]byte("junk")); err != nil {
		t.Fatalf("write __MACOSX entry: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	return buffer.Bytes()
}
