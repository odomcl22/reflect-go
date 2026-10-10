package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var localHTTP = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("local API redirects are refused") }}

type Connection struct {
	Base    string `json:"base_url"`
	ID      string `json:"model"`
	Gateway bool   `json:"gateway"`
	Context int    `json:"context"`
	Model   string `json:"model_file"`
	Backend string `json:"backend"`
	Device  string `json:"device"`
}

func connection(root string) (Connection, string, error) {
	var c Connection
	path, err := statePath(root, "provider.json")
	if err != nil {
		return c, "", err
	}
	if err = readJSON(path, &c); err != nil {
		return c, "", errors.New("start the provider and wait for Engine ready first")
	}
	u, err := url.Parse(c.Base)
	if err != nil {
		return c, "", err
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1024 || port > 65535 || c.Base != fmt.Sprintf("http://127.0.0.1:%d", port) || c.Context < 512 || c.Context > 32768 {
		return c, "", errors.New("invalid local provider settings")
	}
	keyPath, err := statePath(root, "provider-key")
	if err != nil {
		return c, "", err
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return c, "", err
	}
	key := strings.TrimSpace(string(data))
	decoded, e := hex.DecodeString(key)
	if e != nil || len(decoded) != 32 {
		return c, "", errors.New("invalid provider key")
	}
	return c, key, nil
}
func localRequest(root, method, path string, body io.Reader) (*http.Response, error) {
	c, key, err := connection(root)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return localHTTP.Do(req)
}
func findClient(name string) (string, error) {
	if !validClient(name) {
		return "", errors.New("choose Codex, Claude Code or OpenCode")
	}
	if path, err := exec.LookPath(name); err == nil {
		if runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(path), ".exe") {
			return path, nil
		}
	}
	home, _ := os.UserHomeDir()
	candidates := []string{filepath.Join(home, ".local", "bin", name), filepath.Join("/opt/homebrew/bin", name), filepath.Join("/usr/local/bin", name)}
	if runtime.GOOS == "windows" {
		candidates = []string{filepath.Join(home, ".local", "bin", name+".exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", name, name+".exe")}
	}
	for _, path := range candidates {
		if f, err := os.Stat(path); err == nil && !f.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s native CLI not found on this computer", name)
}
func cleanClientEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "OPENAI_") || strings.HasPrefix(key, "CLAUDE_CODE_USE_") || key == "CODEX_API_KEY" || key == "REFLECT_GO_API_KEY" || key == "HTTP_PROXY" || key == "HTTPS_PROXY" || key == "ALL_PROXY" || key == "NO_PROXY" {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "NO_PROXY=127.0.0.1,localhost")
}
func connectClient(root string, args []string) (result error) {
	if len(args) == 0 {
		return errors.New("usage: reflectgo connect codex|claude|opencode --project PATH [--task TEXT] [--source portable|host]")
	}
	name := args[0]
	if !validClient(name) {
		return errors.New("choose Codex, Claude Code or OpenCode")
	}
	stage := "reading launch options"
	defer func() { recordClientLaunch(root, name, stage, result) }()
	flags := flag.NewFlagSet("connect", flag.ContinueOnError)
	project := flags.String("project", "", "")
	task := flags.String("task", "", "")
	source := flags.String("source", "portable", "")
	hold := flags.Bool("hold", false, "")
	profileName := flags.String("launch-profile", "", "")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *hold {
		stage = "attaching terminal"
		if err := prepareClientTerminal(); err != nil {
			return err
		}
		defer func() {
			// Save before waiting for Return, so diagnostics are available while
			// the error window is still open. Never pipe the CLI's terminal I/O.
			recordClientLaunch(root, name, stage, result)
			if result != nil {
				fmt.Fprintln(os.Stderr, "Reflect Go:", result)
			}
			fmt.Printf("Launch report: state/client-launch-%s.json\n", name)
			fmt.Println("Session ended. Press Return to close.")
			interactiveTask()
		}()
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected connector argument")
	}
	stage = "checking project folder"
	dir, err := resolveProject(root, *project)
	if err != nil {
		return err
	}
	stage = "checking client pack and local API"
	var profile *ClientLaunchPreference
	if *profileName != "" {
		p, err := readLaunchProfile(root, *profileName)
		if err != nil {
			return err
		}
		profile = &p
	}
	cmd, err := clientCommandWithProfile(root, name, *source, dir, *task, profile)
	if err != nil {
		return err
	}
	stage = "starting client"
	recordClientLaunch(root, name, stage, nil)
	if err = cmd.Start(); err != nil {
		return err
	}
	stage = "client running"
	recordClientLaunch(root, name, stage, nil)
	result = cmd.Wait()
	stage = "client exited"
	return result
}

func recordClientLaunch(root, name, stage string, result error) {
	if !validClient(name) {
		return
	}
	path, err := statePath(root, "client-launch-"+name+".json")
	if err != nil {
		return
	}
	record := map[string]any{"client": name, "platform": platformID(), "stage": stage, "time_utc": time.Now().UTC().Format(time.RFC3339)}
	if result != nil {
		record["error"] = secretPattern.ReplaceAllString(result.Error(), "[redacted]")
		var exit *exec.ExitError
		if errors.As(result, &exit) {
			record["exit_code"] = exit.ExitCode()
		}
	}
	_ = writeJSON(path, record)
}

func clientCommand(root, name, source, dir, task string) (*exec.Cmd, error) {
	return clientCommandWithProfile(root, name, source, dir, task, nil)
}
func clientCommandWithProfile(root, name, source, dir, task string, profile *ClientLaunchPreference) (*exec.Cmd, error) {
	var err error
	var client string
	if source == "portable" {
		fmt.Println("Checking portable " + name + " pack…")
		client, err = portableClient(root, name, true)
	} else if source == "host" {
		client, err = findClient(name)
	} else {
		return nil, errors.New("source must be portable or host")
	}
	if err != nil {
		return nil, err
	}
	c, key, err := connection(root)
	if err != nil {
		return nil, err
	}
	if name == "opencode" && c.Context < 16384 {
		return nil, errors.New("OpenCode needs at least 16,384 context tokens in this preview; its prompt and compaction overhead can loop at 8,192. Stop the engine and apply Coding settings, or use Chat with a smaller context")
	}
	if c.Context < 8192 {
		return nil, errors.New("coding clients need at least 8,192 context tokens; stop the engine and select 16,384 when memory allows")
	}
	response, err := localRequest(root, "GET", "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("local provider rejected the connection key (HTTP %d); stop and restart the engine in this phase, then reopen the client", response.StatusCode)
		}
		return nil, fmt.Errorf("local provider is not ready (HTTP %d)", response.StatusCode)
	}
	models, err := clientModels(response.Body, c)
	if err != nil {
		return nil, err
	}
	if c.Gateway {
		if profile == nil {
			p, err := savedClientPreference(root, name)
			if err != nil {
				return nil, err
			}
			profile = &p
		}
		models, c.ID, err = selectedClientModels(models, c.ID, *profile)
		if err != nil {
			return nil, err
		}
	}
	env, err := clientEnvironment(root, name)
	if err != nil {
		return nil, err
	}
	var options []string
	if name == "codex" {

		env = append(env, "REFLECT_GO_API_KEY="+key)
		options = []string{"--sandbox", "workspace-write", "-C", dir}
		if task != "" {
			options = append([]string{"exec", "--ephemeral", "--skip-git-repo-check", "--color", "never"}, options...)
		} else {
			options = append(options, "--no-alt-screen")
		}
		configs := []string{`model="reflect-go-local"`, `model_provider="reflect_go"`, `model_providers.reflect_go.name="Reflect Go"`, "model_providers.reflect_go.base_url=" + strconv.Quote(c.Base+"/v1"), `model_providers.reflect_go.env_key="REFLECT_GO_API_KEY"`, `model_providers.reflect_go.wire_api="responses"`, `model_providers.reflect_go.requires_openai_auth=false`, `model_providers.reflect_go.supports_websockets=false`, `model_providers.reflect_go.request_max_retries=0`, `model_providers.reflect_go.stream_max_retries=0`, `model_providers.reflect_go.stream_idle_timeout_ms=180000`, fmt.Sprintf("model_context_window=%d", c.Context), fmt.Sprintf("model_auto_compact_token_limit=%d", c.Context*85/100), "model_instructions_file=" + strconv.Quote(filepath.Join(root, "config", "codex-local-instructions.txt")), `web_search="disabled"`, `check_for_update_on_startup=false`, `analytics.enabled=false`, `approval_policy="on-request"`, `features.unified_exec=false`, `features.plugins=false`, `features.apps=false`, `features.multi_agent=false`, `shell_environment_policy.inherit="core"`, `shell_environment_policy.ignore_default_excludes=false`, `allow_login_shell=false`}
		for _, c := range configs {
			options = append(options, "-c", c)
		}
		if task != "" {
			options = append(options, "--", task)
		}
	} else if name == "claude" {
		env = append(env, "ANTHROPIC_API_KEY="+key, "ANTHROPIC_BASE_URL="+c.Base, "ANTHROPIC_MODEL=reflect-go-local", "ANTHROPIC_DEFAULT_HAIKU_MODEL=reflect-go-local", "ANTHROPIC_DEFAULT_SONNET_MODEL=reflect-go-local", "ANTHROPIC_DEFAULT_OPUS_MODEL=reflect-go-local", "ANTHROPIC_SMALL_FAST_MODEL=reflect-go-local", "DISABLE_PROMPT_CACHING=1", "CLAUDE_CODE_MAX_OUTPUT_TOKENS=2048", "CLAUDE_CODE_MAX_CONTEXT_TOKENS="+strconv.Itoa(c.Context), "MAX_THINKING_TOKENS=0")
		instructions, err := os.ReadFile(filepath.Join(root, "config", "claude-local-instructions.txt"))
		if err != nil {
			return nil, err
		}
		options = []string{"--bare", "--setting-sources", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--disable-slash-commands", "--no-chrome", "--model", "reflect-go-local", "--append-system-prompt", string(instructions), "--tools", "Read,Edit,Write,Bash,PowerShell,Glob,Grep", "--permission-mode", "manual"}
		if task != "" {
			options = append(options, "--print", task)
		}
	} else {
		env = append(env, "REFLECT_GO_API_KEY="+key)
		cfg := map[string]any{"$schema": "https://opencode.ai/config.json", "model": "reflect_go/reflect-go-local", "small_model": "reflect_go/reflect-go-local", "enabled_providers": []string{"reflect_go"}, "autoupdate": false, "share": "disabled", "snapshot": false, "plugin": []string{}, "mcp": map[string]any{}, "lsp": false, "formatter": false, "permission": map[string]string{"*": "ask", "read": "allow", "glob": "allow", "grep": "allow"}, "provider": map[string]any{"reflect_go": map[string]any{"name": "Reflect Go", "npm": "@ai-sdk/openai-compatible", "options": map[string]any{"baseURL": c.Base + "/v1", "apiKey": "{env:REFLECT_GO_API_KEY}"}, "models": map[string]any{"reflect-go-local": map[string]any{"name": c.Model, "tool_call": true, "limit": map[string]int{"context": c.Context, "output": 2048}}}}}}
		data, e := json.Marshal(cfg)
		if e != nil {
			return nil, e
		}
		env = append(env, "OPENCODE_CONFIG_CONTENT="+string(data))
		if task != "" {
			options = []string{"run", "--model", "reflect_go/reflect-go-local", task}
		}
	}
	if err := configureCatalog(root, name, c, models, &options, &env); err != nil {
		return nil, err
	}
	fmt.Printf("%s → Reflect Go (%s) · %s\n", name, c.Base, dir)
	fmt.Println("Client software: " + source + ". Settings and sessions stay on this drive. Use one client at a time.")
	if name == "claude" {
		fmt.Println("If Claude asks to use the supplied local API key, choose Yes for this connection.")
	}
	cmd := exec.Command(client, options...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// The default workspace is separate from runtime/model files and still travels.
func resolveProject(root, project string) (string, error) {
	if project == "" {
		workspace, err := safePath(filepath.Dir(root), "Workspace")
		if err != nil {
			return "", err
		}
		if err = os.MkdirAll(workspace, 0700); err != nil {
			return "", err
		}
		return workspace, nil
	}
	dir, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("project folder does not exist")
	}
	collection := filepath.Dir(root)
	workspace := filepath.Join(collection, "Workspace")
	if (dir == collection || contained(dir, collection)) && dir != workspace && !contained(dir, workspace) {
		return "", errors.New("choose a project outside the runtime/model collection, or use its Workspace folder")
	}
	return dir, nil
}
