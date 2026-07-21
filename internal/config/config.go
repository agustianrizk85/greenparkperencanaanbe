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
	UploadDir   string // board attachment files (one file per attachment ID)

	// Deep Revisi AI (GK Kontraktor vs GK TTD vision check). The Ollama KEY is
	// NOT held here — vision calls are proxied through the auth service, which
	// owns the ONE central key (Panel Admin → Kunci AI). Perencanaan only picks
	// the vision MODEL and shells out to the Python render/annotate scripts.
	OllamaModel  string // default vision model id (overridable at runtime from UI)
	PythonBin    string // interpreter used to shell out to scripts/gk/*.py
	GKScriptsDir string // folder holding render_pages.py / annotate.py
	GKSkillPath  string // dashboard/skillmd/pengecekan-gambar-kerja.md, hot-editable
	AuthAPIBase  string // auth service base URL (…/api) for the vision proxy
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		Port:        getenv("PERENCANAAN_PORT", "8082"),
		AllowOrigin: getenv("PERENCANAAN_ALLOW_ORIGIN", "*"),
		SeedDemo:    getenv("PERENCANAAN_SEED_DEMO", "true") != "false",
		UploadDir:   getenv("PERENCANAAN_UPLOAD_DIR", "data/uploads"),

		OllamaModel:  getenv("OLLAMA_VISION_MODEL", "qwen3.5:397b"),
		PythonBin:    getenv("PYTHON_BIN", defaultPythonBin()),
		GKScriptsDir: getenv("GK_SCRIPTS_DIR", "scripts/gk"),
		GKSkillPath:  getenv("GK_SKILL_PATH", "../../dashboard/skillmd/pengecekan-gambar-kerja.md"),
		AuthAPIBase:  getenv("AUTH_API_BASE", "http://127.0.0.1:8090/api"),
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
