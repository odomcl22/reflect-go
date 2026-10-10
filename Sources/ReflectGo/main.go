package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func parseOptions(args []string) (Options, error) {
	o := Options{Command: "ui", Backend: "auto", Port: 8766, Temperature: -1}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		o.Command = args[0]
		args = args[1:]
	}
	switch o.Command {
	case "ui", "chat", "run", "serve", "gateway", "inventory", "models", "doctor", "verify", "qualify":
	default:
		return o, fmt.Errorf("unknown command: %s", o.Command)
	}
	f := flag.NewFlagSet("Reflect Go", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.EnabledIDs, "enabled-ids", "", "")
	f.StringVar(&o.Model, "model", "", "")
	f.StringVar(&o.Prompt, "prompt", "", "")
	f.StringVar(&o.Backend, "backend", "auto", "")
	f.IntVar(&o.Context, "context", 0, "")
	f.IntVar(&o.MaxTokens, "max-tokens", 0, "")
	f.IntVar(&o.Threads, "threads", 0, "")
	f.IntVar(&o.Port, "port", 8766, "")
	f.Float64Var(&o.Temperature, "temperature", -1, "")
	f.BoolVar(&o.CPU, "cpu", false, "")
	f.BoolVar(&o.Internal, "internal", false, "")
	f.BoolVar(&o.Verbose, "verbose", false, "")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, errors.New("unexpected positional argument")
	}
	visited := map[string]bool{}
	f.Visit(func(v *flag.Flag) { visited[v.Name] = true })
	if visited["context"] && (o.Context < 512 || o.Context > 32768) {
		return o, errors.New("context must be 512...32768")
	}
	if visited["max-tokens"] && o.MaxTokens <= 0 {
		return o, errors.New("max-tokens must be positive")
	}
	if visited["threads"] && (o.Threads < 1 || o.Threads > 256) {
		return o, errors.New("threads must be 1...256")
	}
	if o.Port < 1024 || o.Port > 65535 {
		return o, errors.New("port must be 1024...65535")
	}
	if visited["temperature"] && (math.IsNaN(o.Temperature) || math.IsInf(o.Temperature, 0) || o.Temperature < 0 || o.Temperature > 2) {
		return o, errors.New("temperature must be 0...2")
	}
	if o.Backend != "auto" && o.Backend != "cpu" && o.Backend != "metal" && o.Backend != "vulkan" {
		return o, errors.New("backend must be auto, cpu, metal or vulkan")
	}
	if o.Command == "run" && o.Prompt == "" {
		return o, errors.New("run requires --prompt TEXT")
	}
	if o.Command != "run" && visited["prompt"] {
		return o, errors.New("prompt is only accepted with run")
	}
	if o.Command == "serve" && visited["max-tokens"] {
		return o, errors.New("provider clients set their own output budget")
	}
	return o, nil
}
func emit(v any) error { e := json.NewEncoder(os.Stdout); e.SetIndent("", "  "); return e.Encode(v) }
func main() {
	if len(os.Args) > 1 && (os.Args[1] == "gateway" || os.Args[1] == "serve" || os.Args[1] == "run" || os.Args[1] == "chat" || os.Args[1] == "_supervise" || os.Args[1] == "connect") {
		if err := setupProcessTree(); err != nil {
			showLaunchError(err)
			fmt.Fprintln(os.Stderr, "Reflect Go:", err)
			os.Exit(1)
		}
	}
	if err := mainRun(os.Args[1:]); err != nil {
		showLaunchError(err)
		fmt.Fprintln(os.Stderr, "Reflect Go:", err)
		os.Exit(1)
	}
}
func mainRun(args []string) error {
	if len(args) > 0 && args[0] == "_supervise" {
		return supervise(args[1:])
	}
	root, err := rootDirectory()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "connect" {
		return connectClient(root, args[1:])
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "help") {
		fmt.Println("Reflect Go 0.4.5\nCommands: ui, chat, run --prompt TEXT, serve, inventory, models, doctor, verify, qualify, connect\nOptions: --model FILE --context N --cpu --backend auto|cpu|metal|vulkan --threads N --temperature N --port N\nKeep the whole Phase 4.5 folder beside Models. No runtime downloads or cloud fallback.")
		return nil
	}
	o, err := parseOptions(args)
	if err != nil {
		return err
	}
	s, err := loadSettings(root)
	if err != nil {
		return err
	}
	if o.Command == "verify" {
		return verify(root, s, os.Stdout)
	}
	if o.Command == "ui" {
		return serveUI(root)
	}
	if o.Command == "qualify" {
		return qualify(root, s, o)
	}
	if o.Command == "chat" || o.Command == "run" {
		if o.Context == 0 {
			o.Context = s.Context
		}
	}
	inv, err := inventory(root, s, o)
	if err != nil {
		return err
	}
	switch o.Command {
	case "doctor":
		return emit(inv.Hardware)
	case "inventory":
		return emit(inv)
	case "models":
		for _, m := range inv.Models {
			fmt.Printf("%s — %.2f GiB, %.2f GiB estimated, eligible=%v %s\n", m.Name, float64(m.Bytes)/float64(GiB), float64(m.EstimatedBytes)/float64(GiB), m.Eligible, m.Issue)
		}
		return nil
	}
	if o.Threads > inv.Hardware.CPUThreads {
		return errors.New("threads exceeds this host's CPU thread count")
	}
	if o.Command == "gateway" {
		return serveGateway(root, s, o, inv)
	}
	return launch(root, s, o, inv)
}
func chooseModel(inv Inventory, o Options) (Model, error) {
	if o.Model != "" {
		for _, m := range inv.Models {
			if m.Name == o.Model {
				if !m.Eligible {
					return m, fmt.Errorf("selected model is ineligible: %s; estimated %.2f GiB exceeds budget or format check failed", m.Issue, float64(m.EstimatedBytes)/float64(GiB))
				}
				return m, nil
			}
		}
		return Model{}, errors.New("model not found in Models")
	}
	for _, m := range inv.Models {
		if m.Name == inv.DefaultModel && m.Eligible && o.Command == "serve" {
			return m, nil
		}
	}
	for _, m := range inv.Models {
		if m.Eligible {
			return m, nil
		}
	}
	return Model{}, errors.New("no model passes the memory/format screen; lower context or use a smaller compatible GGUF")
}
func launch(root string, s Settings, o Options, inv Inventory) error {
	m, err := chooseModel(inv, o)
	if err != nil {
		return err
	}
	backend, err := backendForModel(root, inv.Hardware.Backend, m, o.Backend, o.CPU)
	if err != nil {
		return err
	}
	hashes, err := manifest(root)
	if err != nil {
		return err
	}
	if expected, ok := hashes["models/"+m.Name]; ok {
		fmt.Fprintln(os.Stderr, "Checking model integrity…")
		actual, err := hashFile(m.Path)
		if err != nil || actual != expected {
			return errors.New("packaged model integrity check failed")
		}
	}
	context := inv.Context
	maxTokens := o.MaxTokens
	if maxTokens == 0 {
		maxTokens = s.MaxTokens
	}
	if maxTokens >= context {
		return errors.New("max-tokens must be smaller than context")
	}
	temp := s.Temperature
	if o.Temperature >= 0 {
		temp = o.Temperature
	}
	threads := o.Threads
	if threads == 0 {
		threads = max(1, inv.Hardware.CPUThreads-2)
	}
	args := []string{"--model", m.Path, "--offline", "--jinja", "--ctx-size", strconv.Itoa(context), "--threads", strconv.Itoa(threads), "--parallel", "1", "--batch-size", "256", "--ubatch-size", "128", "--fit", "off", "--temp", strconv.FormatFloat(temp, 'f', -1, 64), "--seed", "42"}
	if backend.Kind == "cpu" {
		args = append(args, "--device", "none", "--gpu-layers", "0", "--no-kv-offload", "--no-op-offload")
	} else {
		args = append(args, "--device", backend.DeviceID, "--gpu-layers", "all")
	}
	fmt.Fprintf(os.Stderr, "Hardware: %s · %.1f GiB RAM · %s (%s)\n", inv.Hardware.CPU, float64(inv.Hardware.PhysicalMemoryBytes)/float64(GiB), backend.Kind, backend.Device)
	if backend.Reason != "" {
		fmt.Fprintln(os.Stderr, backend.Reason)
	}
	name := "llama-completion"
	var lock *os.File
	if o.Command == "serve" {
		lockName := "provider.lock"
		if o.Internal {
			lockName = "runtime.lock"
		}
		if o.Internal {
			lock, err = lockNamed(root, lockName)
		} else {
			lock, err = lockProvider(root)
		}
		if err != nil {
			return err
		}
		defer lock.Close()
		if err = checkPort(o.Port); err != nil {
			return err
		}
		keyPath, err := providerKey(root)
		if err != nil {
			return err
		}
		base := fmt.Sprintf("http://127.0.0.1:%d", o.Port)
		connectionName := "provider.json"
		if o.Internal {
			connectionName = "runtime-provider.json"
		}
		connection, err := statePath(root, connectionName)
		if err != nil {
			return err
		}
		if err = writeJSON(connection, map[string]any{"base_url": base, "model": "reflect-go-local", "model_file": m.Name, "context": context, "backend": backend.Kind, "device": backend.Device}); err != nil {
			return err
		}
		alias := "reflect-go-local"
		if o.Internal {
			alias = modelID(m.Name)
		}
		args = append(args, "--alias", alias, "--host", "127.0.0.1", "--port", strconv.Itoa(o.Port), "--api-key-file", keyPath, "--reasoning", "off", "--no-webui", "--no-ui-mcp-proxy", "--no-agent", "--metrics", "--no-slots", "--cors-origins", base, "--no-cors-credentials", "--cache-ram", "0", "--ctx-checkpoints", "0", "--verbosity", "3")
		fmt.Fprintln(os.Stderr, "Wait for listening before connecting clients. Ctrl-C stops the provider.")
		name = "llama-server"
	} else {
		args = append(args, "--conversation", "--predict", strconv.Itoa(maxTokens), "--no-context-shift", "--no-escape", "--log-colors", "off", "--system-prompt", "You are a local AI assistant running through Reflect Go. Your weights are on the portable drive; inference runs on this computer's CPU or GPU. This chat has no web browsing or cloud inference. Do not claim to run on the internet.")
		if o.Verbose {
			args = append(args, "--perf", "--verbosity", "5")
		} else {
			args = append(args, "--verbosity", "1")
		}
		if o.Command == "run" {
			args = append(args, "--single-turn", "--prompt", o.Prompt, "--simple-io", "--color", "off", "--no-display-prompt")
		}
	}
	env, err := runtimeEnv(root, backend.Path)
	if err != nil {
		return err
	}
	state, err := safePath(root, "state")
	if err != nil {
		return err
	}
	return runRuntime(runtimeBinary(backend.Path, name), args, env, state)
}
func providerKey(root string) (string, error) {
	path, err := statePath(root, "provider-key")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = make([]byte, 32)
		if _, err = rand.Read(data); err != nil {
			return "", err
		}
		data = []byte(hex.EncodeToString(data) + "\n")
		if err = atomicWrite(path, data, 0600); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(data))
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("invalid provider key; move state/provider-key aside while stopped and retry")
	}
	return path, nil
}
func supervise(args []string) error {
	if len(args) < 1 {
		return errors.New("supervisor needs a command")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, args...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	hideEngineWindow(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Reflect Go engine PID: %d\n", cmd.Process.Pid)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	owner := make(chan struct{})
	go func() { io.Copy(io.Discard, os.Stdin); close(owner) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	select {
	case err := <-done:
		return err
	case <-owner:
	case <-signals:
	}
	cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		return nil
	case <-time.After(8 * time.Second):
		cmd.Process.Kill()
		<-done
		return nil
	}
}
func interactiveTask() string {
	fmt.Fprint(os.Stdout, "Task: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}
