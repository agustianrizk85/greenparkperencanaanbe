// Package config loads runtime configuration from environment variables with
// sensible defaults, so the service runs out of the box for local development.
package config

import (
	"os"
	"runtime"
)

// Config holds the server runtime configuration.
type Config struct {
	Port        string // HTTP port to listen on
	AllowOrigin string // CORS allowed origin
	SeedDemo    bool   // populate sample data on startup

	// Deep Revisi AI (GK Kontraktor vs GK TTD vision check via Ollama Cloud).
	// Self-contained here — deliberately NOT routed through the shared
	// be/auth AI proxy (see plan notes: that service's working tree is
	// currently broken and unrelated to this feature).
	OllamaAPIKey    string // Ollama Cloud API key; feature no-ops without it
	OllamaModel     string // vision-capable model id
	OllamaEndpoint  string // chat completions URL
	PythonBin       string // interpreter used to shell out to scripts/gk/*.py
	GKScriptsDir    string // folder holding render_pages.py / annotate.py
	GKSkillPath     string // dashboard/skillmd/pengecekan-gambar-kerja.md, hot-editable
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Port:        getenv("PERENCANAAN_PORT", "8082"),
		AllowOrigin: getenv("PERENCANAAN_ALLOW_ORIGIN", "*"),
		SeedDemo:    getenv("PERENCANAAN_SEED_DEMO", "true") != "false",

		OllamaAPIKey:   getenv("OLLAMA_API_KEY", ""),
		OllamaModel:    getenv("OLLAMA_VISION_MODEL", "qwen3.5:397b"),
		OllamaEndpoint: getenv("OLLAMA_ENDPOINT", "https://ollama.com/v1/chat/completions"),
		PythonBin:      getenv("PYTHON_BIN", defaultPythonBin()),
		GKScriptsDir:   getenv("GK_SCRIPTS_DIR", "scripts/gk"),
		GKSkillPath:    getenv("GK_SKILL_PATH", "../../dashboard/skillmd/pengecekan-gambar-kerja.md"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// defaultPythonBin picks a sane interpreter name per OS: the "py" launcher on
// Windows dev machines, "python3" on the Linux production VPS.
func defaultPythonBin() string {
	if runtime.GOOS == "windows" {
		return "py"
	}
	return "python3"
}
