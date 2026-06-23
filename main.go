package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval     int              `yaml:"interval"`
	StartupSleep int              `yaml:"startup_sleep"`
	Discover     bool             `yaml:"discover"`
	Verbose      bool             `yaml:"verbose"`
	Checks       map[string]Check `yaml:"checks"`
}

type Check struct {
	URL      string `yaml:"url"`
	OK       []int  `yaml:"ok"`
	Restart  string `yaml:"restart"`
	Failures int    `yaml:"failures"`
	Cooldown int    `yaml:"cooldown"`
}

type CheckState struct {
	Failures    int
	LastRestart time.Time
}

type DockerContainer struct {
	ID     string
	Name   string
	Config struct {
		Labels       map[string]string
		ExposedPorts map[string]any
	}
	HostConfig struct {
		RestartPolicy struct {
			Name string
		}
	}
	State struct {
		Running    bool
		Paused     bool
		Restarting bool
		Dead       bool
	}
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
		cfg.Checks[name] = normalizeCheck(check)
	}

	if cfg.Verbose {
		log("started with verbose logging enabled")
	} else {
		log("started")
	}

	if cfg.StartupSleep > 0 {
		log("sleeping %d seconds before first check", cfg.StartupSleep)
		time.Sleep(time.Duration(cfg.StartupSleep) * time.Second)
	}

	states := map[string]*CheckState{}
	lastMonitored := ""

	for {
		checks := cfg.Checks

		if cfg.Discover {
			discovered, err := discoverChecks()
			if err != nil {
				log("discovery failed: %v", err)
			}

			checks = mergeChecks(cfg.Checks, discovered)
		}

		monitored := monitoredServices(checks)
		if monitored != lastMonitored {
			log("monitoring services: %s", monitored)
			lastMonitored = monitored
		}

		for name, check := range checks {
			if states[name] == nil {
				states[name] = &CheckState{}
			}
			handleCheck(name, check, states[name], cfg.Verbose)
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

func handleCheck(name string, check Check, state *CheckState, verbose bool) {
	code, err := probe(check.URL)

	if err != nil {
		log("[%s] probe failed: %v", name, err)
		handleFailure(name, check, state)
		return
	}

	if contains(check.OK, code) {
		state.Failures = 0
		if verbose {
			log("[%s] healthy, HTTP %d", name, code)
		}
		return
	}

	log("[%s] unhealthy, HTTP %d", name, code)
	handleFailure(name, check, state)
}

func handleFailure(name string, check Check, state *CheckState) {
	state.Failures++

	if state.Failures < check.Failures {
		log("[%s] failure %d/%d", name, state.Failures, check.Failures)
		return
	}

	if check.Cooldown > 0 && !state.LastRestart.IsZero() {
		wait := time.Duration(check.Cooldown)*time.Second - time.Since(state.LastRestart)
		if wait > 0 {
			log("[%s] restart skipped: cooldown active for %s", name, wait.Round(time.Second))
			return
		}
	}

	if restart(check.Restart) {
		state.Failures = 0
		state.LastRestart = time.Now()
	}
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

func restart(container string) bool {
	if container == "" {
		log("restart skipped: no container configured")
		return false
	}

	log("restarting container: %s", container)

	cmd := exec.Command("docker", "restart", container)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log("restart failed for %s: %v; output: %s", container, err, string(output))
		return false
	}

	log("restart successful for %s; output: %s", container, string(output))
	return true
}

func discoverChecks() (map[string]Check, error) {
	output, err := exec.Command("docker", "ps", "-q", "--filter", "label=paracetamol.enable=true").Output()
	if err != nil {
		return nil, err
	}

	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return nil, nil
	}

	args := append([]string{"inspect"}, ids...)
	output, err = exec.Command("docker", args...).Output()
	if err != nil {
		return nil, err
	}

	var containers []DockerContainer
	if err := json.Unmarshal(output, &containers); err != nil {
		return nil, err
	}

	checks := map[string]Check{}

	for _, container := range containers {
		check, ok := checkFromContainer(container)
		if ok {
			name := strings.TrimPrefix(container.Name, "/")
			if service := container.Config.Labels["com.docker.compose.service"]; service != "" {
				name = service
			}
			checks[name] = check
		}
	}

	return checks, nil
}

func checkFromContainer(container DockerContainer) (Check, bool) {
	labels := container.Config.Labels

	if !container.State.Running || container.State.Paused || container.State.Restarting || container.State.Dead {
		return Check{}, false
	}

	if !allowedRestartPolicy(container.HostConfig.RestartPolicy.Name) {
		return Check{}, false
	}

	url := labels["paracetamol.url"]
	if url == "" {
		port := labels["paracetamol.port"]
		if port == "" {
			port = singleExposedPort(container)
		}
		if port == "" {
			return Check{}, false
		}

		host := strings.TrimPrefix(container.Name, "/")
		if service := labels["com.docker.compose.service"]; service != "" {
			host = service
		}

		protocol := valueOrDefault(labels["paracetamol.protocol"], "http")
		path := valueOrDefault(labels["paracetamol.path"], "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		url = fmt.Sprintf("%s://%s:%s%s", protocol, host, port, path)
	}

	return normalizeCheck(Check{
		URL:        url,
		OK:         parseIntList(labels["paracetamol.ok"]),
		Restart:    container.ID,
		Failures:   parseInt(labels["paracetamol.failures"], 3),
		Cooldown:   parseInt(labels["paracetamol.cooldown"], 300),
	}), true
}

func mergeChecks(manual, discovered map[string]Check) map[string]Check {
	checks := map[string]Check{}

	for name, check := range discovered {
		checks[name] = check
	}

	for name, check := range manual {
		checks[name] = check
	}

	return checks
}

func monitoredServices(checks map[string]Check) string {
	if len(checks) == 0 {
		return "none"
	}

	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}

	sort.Strings(names)
	return strings.Join(names, ", ")
}

func normalizeCheck(check Check) Check {
	if len(check.OK) == 0 {
		check.OK = []int{http.StatusOK}
	}

	if check.Failures <= 0 {
		check.Failures = 1
	}

	return check
}

func allowedRestartPolicy(policy string) bool {
	return policy == "always" || policy == "unless-stopped"
}

func singleExposedPort(container DockerContainer) string {
	var port string

	for exposed := range container.Config.ExposedPorts {
		parts := strings.Split(exposed, "/")
		if len(parts) != 2 || parts[1] != "tcp" {
			continue
		}

		if port != "" {
			return ""
		}

		port = parts[0]
	}

	return port
}

func parseInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func parseIntList(raw string) []int {
	var values []int

	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil {
			values = append(values, value)
		}
	}

	return values
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
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
