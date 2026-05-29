package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval     int              `yaml:"interval"`
	StartupSleep int              `yaml:"startup_sleep"`
	Checks       map[string]Check `yaml:"checks"`
}

type Check struct {
	URL     string `yaml:"url"`
	OK      []int  `yaml:"ok"`
	Restart string `yaml:"restart"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		panic(err)
	}

	if cfg.Interval == 0 {
		cfg.Interval = 60
	}

	for name, check := range cfg.Checks {
		if len(check.OK) == 0 {
			check.OK = []int{http.StatusOK}
			cfg.Checks[name] = check
		}
	}

	log("started")

	if cfg.StartupSleep > 0 {
		log("sleeping %d seconds before first check", cfg.StartupSleep)
		time.Sleep(time.Duration(cfg.StartupSleep) * time.Second)
	}

	for {
		for name, check := range cfg.Checks {
			handleCheck(name, check)
		}

		time.Sleep(time.Duration(cfg.Interval) * time.Second)
	}
}

func loadConfig() (Config, error) {
	raw := os.Getenv("CONFIG")

	if raw == "" {
		bytes, err := os.ReadFile("/config.yml")
		if err != nil {
			return Config{}, fmt.Errorf("CONFIG env missing and /config.yml not readable: %w", err)
		}
		raw = string(bytes)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func handleCheck(name string, check Check) {
	code, err := probe(check.URL)

	if err != nil {
		log("[%s] probe failed: %v", name, err)
		restart(check.Restart)
		return
	}

	if contains(check.OK, code) {
		log("[%s] healthy, HTTP %d", name, code)
		return
	}

	log("[%s] unhealthy, HTTP %d", name, code)
	restart(check.Restart)
}

func probe(url string) (int, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func restart(container string) {
	if container == "" {
		log("restart skipped: no container configured")
		return
	}

	log("restarting container: %s", container)

	cmd := exec.Command("docker", "restart", container)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log("restart failed for %s: %v; output: %s", container, err, string(output))
		return
	}

	log("restart successful for %s; output: %s", container, string(output))
}

func contains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func log(format string, args ...any) {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [paracetamol] %s\n", now, fmt.Sprintf(format, args...))
}
