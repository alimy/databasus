package config

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"databasus-verification-agent/internal/logger"
)

var log = logger.GetLogger()

const configFileName = "databasus-verification.json"

const minRAMMbPerJob = 512

type Config struct {
	DatabasusHost     string `json:"databasusHost"`
	AgentID           string `json:"agentId"`
	Token             string `json:"token"`
	MaxCPU            int    `json:"maxCpu"`
	MaxRAMMb          int    `json:"maxRamMb"`
	MaxDiskGb         int    `json:"maxDiskGb"`
	MaxConcurrentJobs int    `json:"maxConcurrentJobs"`
	AllowInsecureHTTP bool   `json:"allowInsecureHttp"`

	flags parsedFlags
}

type Capacity struct {
	MaxCPU            int
	MaxRAMMb          int
	MaxDiskGb         int
	MaxConcurrentJobs int

	CPUPerJob   int
	RAMMbPerJob int
}

// LoadFromJSONAndArgs reads databasus-verification.json into the struct
// and overrides JSON values with any explicitly provided CLI flags.
func (c *Config) LoadFromJSONAndArgs(fs *flag.FlagSet, args []string) {
	c.loadFromJSON()
	c.initSources()

	c.flags.databasusHost = fs.String(
		"databasus-host",
		"",
		"Databasus server URL (e.g. https://your-server:4005)",
	)
	c.flags.agentID = fs.String("agent-id", "", "Verification agent ID")
	c.flags.token = fs.String("token", "", "Verification agent token")
	c.flags.maxCPU = fs.Int("max-cpu", 0, "Total CPU cores available to the agent")
	c.flags.maxRAMMb = fs.Int("max-ram-mb", 0, "Total RAM in MB available to the agent")
	c.flags.maxDiskGb = fs.Int("max-disk-gb", 0, "Total scratch disk in GB available to the agent")
	c.flags.maxConcurrentJobs = fs.Int("max-concurrent-jobs", 0, "Number of verifications to run in parallel")
	c.flags.allowInsecureHTTP = fs.Bool(
		"allow-insecure-http",
		false,
		"Permit a plain http:// databasus-host (token and backup data sent unencrypted)",
	)

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	c.applyFlags()
	log.Info("========= Loading config ============")
	c.logConfigSources()
	log.Info("========= Config has been loaded ====")
}

// SaveToJSON writes the current struct to databasus-verification.json.
func (c *Config) SaveToJSON() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFileName, data, 0o644)
}

// LoadFromJSON reads databasus-verification.json without applying CLI flags.
// The detached _run daemon uses this: the parent `start` already validated and
// persisted the config, so the child trusts the file as-is.
func (c *Config) LoadFromJSON() {
	c.loadFromJSON()
}

func (c *Config) Validate() (Capacity, error) {
	if c.DatabasusHost == "" {
		return Capacity{}, fmt.Errorf("databasus-host is required")
	}

	if c.AgentID == "" {
		return Capacity{}, fmt.Errorf("agent-id is required")
	}

	if c.Token == "" {
		return Capacity{}, fmt.Errorf("token is required")
	}

	return c.DeriveCapacity()
}

func (c *Config) DeriveCapacity() (Capacity, error) {
	if c.MaxConcurrentJobs < 1 {
		return Capacity{}, fmt.Errorf(
			"max-concurrent-jobs must be >= 1 (got %d)", c.MaxConcurrentJobs)
	}

	if c.MaxCPU < 1 {
		return Capacity{}, fmt.Errorf("max-cpu must be >= 1 (got %d)", c.MaxCPU)
	}

	if c.MaxDiskGb < 1 {
		return Capacity{}, fmt.Errorf("max-disk-gb must be >= 1 (got %d)", c.MaxDiskGb)
	}

	if c.MaxRAMMb < minRAMMbPerJob {
		return Capacity{}, fmt.Errorf(
			"max-ram-mb must be >= %d (got %d)", minRAMMbPerJob, c.MaxRAMMb)
	}

	cpuPerJob := c.MaxCPU / c.MaxConcurrentJobs
	ramMbPerJob := c.MaxRAMMb / c.MaxConcurrentJobs

	if cpuPerJob < 1 {
		return Capacity{}, fmt.Errorf(
			"max-cpu (%d) split across max-concurrent-jobs (%d) yields < 1 CPU per job; "+
				"lower max-concurrent-jobs or raise max-cpu",
			c.MaxCPU, c.MaxConcurrentJobs)
	}

	if ramMbPerJob < minRAMMbPerJob {
		return Capacity{}, fmt.Errorf(
			"max-ram-mb (%d) split across max-concurrent-jobs (%d) yields %d MB per job, "+
				"below the %d MB floor; lower max-concurrent-jobs or raise max-ram-mb",
			c.MaxRAMMb, c.MaxConcurrentJobs, ramMbPerJob, minRAMMbPerJob)
	}

	return Capacity{
		MaxCPU:            c.MaxCPU,
		MaxRAMMb:          c.MaxRAMMb,
		MaxDiskGb:         c.MaxDiskGb,
		MaxConcurrentJobs: c.MaxConcurrentJobs,
		CPUPerJob:         cpuPerJob,
		RAMMbPerJob:       ramMbPerJob,
	}, nil
}

// ValidateTransport enforces the http/https gate before any goroutine starts.
// The per-agent token and the decrypted backup stream both cross this link, so
// plain HTTP is allowed only with explicit operator consent.
func (c *Config) ValidateTransport(isStdinTTY bool, in *bufio.Reader) error {
	parsed, err := url.Parse(c.DatabasusHost)
	if err != nil {
		return fmt.Errorf("databasus-host is not a valid URL: %w", err)
	}

	switch parsed.Scheme {
	case "https":
		return nil

	case "http":
		return c.consentToInsecureHTTP(isStdinTTY, in)

	default:
		return fmt.Errorf(
			"databasus-host must start with https:// or http:// (got scheme %q)", parsed.Scheme)
	}
}

func (c *Config) loadFromJSON() {
	data, err := os.ReadFile(configFileName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("No databasus-verification.json found, will create on save")
			return
		}

		log.Warn("Failed to read databasus-verification.json", "error", err)

		return
	}

	if err := json.Unmarshal(data, c); err != nil {
		log.Warn("Failed to parse databasus-verification.json", "error", err)

		return
	}

	log.Info("Configuration loaded from " + configFileName)
}

func (c *Config) initSources() {
	c.flags.sources = map[string]string{
		"databasus-host":      "not configured",
		"agent-id":            "not configured",
		"token":               "not configured",
		"max-cpu":             "not configured",
		"max-ram-mb":          "not configured",
		"max-disk-gb":         "not configured",
		"max-concurrent-jobs": "not configured",
		"allow-insecure-http": "not configured",
	}

	if c.DatabasusHost != "" {
		c.flags.sources["databasus-host"] = configFileName
	}

	if c.AgentID != "" {
		c.flags.sources["agent-id"] = configFileName
	}

	if c.Token != "" {
		c.flags.sources["token"] = configFileName
	}

	if c.MaxCPU != 0 {
		c.flags.sources["max-cpu"] = configFileName
	}

	if c.MaxRAMMb != 0 {
		c.flags.sources["max-ram-mb"] = configFileName
	}

	if c.MaxDiskGb != 0 {
		c.flags.sources["max-disk-gb"] = configFileName
	}

	if c.MaxConcurrentJobs != 0 {
		c.flags.sources["max-concurrent-jobs"] = configFileName
	}

	if c.AllowInsecureHTTP {
		c.flags.sources["allow-insecure-http"] = configFileName
	}
}

func (c *Config) applyFlags() {
	if c.flags.databasusHost != nil && *c.flags.databasusHost != "" {
		c.DatabasusHost = *c.flags.databasusHost
		c.flags.sources["databasus-host"] = "command line args"
	}

	if c.flags.agentID != nil && *c.flags.agentID != "" {
		c.AgentID = *c.flags.agentID
		c.flags.sources["agent-id"] = "command line args"
	}

	if c.flags.token != nil && *c.flags.token != "" {
		c.Token = *c.flags.token
		c.flags.sources["token"] = "command line args"
	}

	if c.flags.maxCPU != nil && *c.flags.maxCPU != 0 {
		c.MaxCPU = *c.flags.maxCPU
		c.flags.sources["max-cpu"] = "command line args"
	}

	if c.flags.maxRAMMb != nil && *c.flags.maxRAMMb != 0 {
		c.MaxRAMMb = *c.flags.maxRAMMb
		c.flags.sources["max-ram-mb"] = "command line args"
	}

	if c.flags.maxDiskGb != nil && *c.flags.maxDiskGb != 0 {
		c.MaxDiskGb = *c.flags.maxDiskGb
		c.flags.sources["max-disk-gb"] = "command line args"
	}

	if c.flags.maxConcurrentJobs != nil && *c.flags.maxConcurrentJobs != 0 {
		c.MaxConcurrentJobs = *c.flags.maxConcurrentJobs
		c.flags.sources["max-concurrent-jobs"] = "command line args"
	}

	// --allow-insecure-http is a presence flag: passing it (or a persisted
	// true) latches on; it is never turned back off from the CLI so a
	// restart under systemd does not re-prompt.
	if c.flags.allowInsecureHTTP != nil && *c.flags.allowInsecureHTTP {
		c.AllowInsecureHTTP = true
		c.flags.sources["allow-insecure-http"] = "command line args"
	}
}

func (c *Config) logConfigSources() {
	log.Info("databasus-host", "value", c.DatabasusHost, "source", c.flags.sources["databasus-host"])
	log.Info("agent-id", "value", c.AgentID, "source", c.flags.sources["agent-id"])
	log.Info("token", "value", maskSensitive(c.Token), "source", c.flags.sources["token"])
	log.Info("max-cpu", "value", c.MaxCPU, "source", c.flags.sources["max-cpu"])
	log.Info("max-ram-mb", "value", c.MaxRAMMb, "source", c.flags.sources["max-ram-mb"])
	log.Info("max-disk-gb", "value", c.MaxDiskGb, "source", c.flags.sources["max-disk-gb"])
	log.Info(
		"max-concurrent-jobs",
		"value",
		c.MaxConcurrentJobs,
		"source",
		c.flags.sources["max-concurrent-jobs"],
	)
	log.Info(
		"allow-insecure-http",
		"value",
		fmt.Sprintf("%v", c.AllowInsecureHTTP),
		"source",
		c.flags.sources["allow-insecure-http"],
	)
}

func (c *Config) consentToInsecureHTTP(isStdinTTY bool, in *bufio.Reader) error {
	if c.AllowInsecureHTTP {
		log.Warn("databasus-host is plain HTTP; transport is unencrypted")

		return nil
	}

	if !isStdinTTY {
		return fmt.Errorf(
			"refusing to use plain HTTP over a non-interactive connection: " +
				"switch --databasus-host to https:// or pass --allow-insecure-http " +
				"to accept unencrypted transport")
	}

	fmt.Fprint(os.Stderr,
		"WARNING: connecting to the primary over plain HTTP, not HTTPS. "+
			"The agent token and decrypted backup data are sent unencrypted. "+
			"Continue? [y/N] ")

	answer, _ := in.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))

	if answer != "y" && answer != "yes" {
		return fmt.Errorf("aborted: plain HTTP transport declined by operator")
	}

	return nil
}

func maskSensitive(value string) string {
	if value == "" {
		return "(not set)"
	}

	visibleLen := max(len(value)/4, 1)

	return value[:visibleLen] + "***"
}
