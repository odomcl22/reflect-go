import Foundation
import Metal
import CryptoKit
import MachO
import Darwin
import Security

// No packages: this executable uses only frameworks shipped with macOS.
let fm = FileManager.default
let gib = Double(1 << 30)

struct Failure: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

func note(_ text: String) {
    FileHandle.standardError.write(Data((text + "\n").utf8))
}

func rootDirectory() throws -> URL {
    var size: UInt32 = 0
    _NSGetExecutablePath(nil, &size)
    var bytes = [CChar](repeating: 0, count: Int(size))
    guard _NSGetExecutablePath(&bytes, &size) == 0 else { throw Failure("Cannot locate PortableAI.") }
    return URL(fileURLWithPath: String(cString: bytes)).resolvingSymlinksInPath()
        .deletingLastPathComponent().deletingLastPathComponent()
}

func contained(_ url: URL, in root: URL) -> Bool {
    url.resolvingSymlinksInPath().path.hasPrefix(root.resolvingSymlinksInPath().path + "/")
}

func sysctlString(_ key: String) -> String {
    var size = 0
    guard sysctlbyname(key, nil, &size, nil, 0) == 0, size > 0 else { return "unknown" }
    var bytes = [CChar](repeating: 0, count: size)
    guard sysctlbyname(key, &bytes, &size, nil, 0) == 0 else { return "unknown" }
    return String(cString: bytes)
}

struct Hardware: Encodable {
    let os = ProcessInfo.processInfo.operatingSystemVersionString
    let cpu = sysctlString("machdep.cpu.brand_string")
    let architecture = "arm64"
    let physicalMemoryBytes = ProcessInfo.processInfo.physicalMemory
    let cpuThreads = ProcessInfo.processInfo.activeProcessorCount
    let metalDevice: String?
    let metalRecommendedBytes: UInt64?
    let screeningBudgetBytes: UInt64

    init() {
        let device = MTLCreateSystemDefaultDevice()
        metalDevice = device?.name
        metalRecommendedBytes = device?.recommendedMaxWorkingSetSize
        // This intentionally reserves half the physical memory for macOS/other apps.
        // It is a static screen, NOT a measurement of currently available memory.
        screeningBudgetBytes = min(physicalMemoryBytes / 2,
                                   device?.recommendedMaxWorkingSetSize ?? physicalMemoryBytes / 2)
    }
}

struct Settings: Decodable {
    var context: Int
    var max_tokens: Int
    var temperature: Double
    var models_directory: String?
    var provider_context: Int?
    var provider_model: String?

    func validate() throws {
        guard (512...32768).contains(context) else { throw Failure("context must be 512...32768.") }
        guard max_tokens > 0, max_tokens < context else {
            throw Failure("max_tokens must be positive and smaller than context.")
        }
        guard temperature.isFinite, (0...2).contains(temperature) else {
            throw Failure("temperature must be between 0 and 2.")
        }
        guard ["models", "../Models"].contains(models_directory ?? "models") else {
            throw Failure("models_directory must be models or ../Models.")
        }
    }
}

struct Options {
    var command = "chat"
    var model: String?
    var prompt: String?
    var cpu = false
    var verbose = false
    var context: Int?
    var maxTokens: Int?
    var port = 8765
    var temperature: Double?
    var threads: Int?

    init(_ arguments: [String]) throws {
        var args = arguments
        if let first = args.first, !first.hasPrefix("--") { command = args.removeFirst() }
        guard ["chat", "run", "serve", "doctor", "inventory", "models", "verify"].contains(command) else {
            throw Failure("Unknown command: \(command). Use --help.")
        }
        while !args.isEmpty {
            let flag = args.removeFirst()
            if flag == "--cpu" { cpu = true; continue }
            if flag == "--verbose" { verbose = true; continue }
            guard ["--model", "--prompt", "--context", "--max-tokens", "--port", "--temperature", "--threads"].contains(flag),
                  !args.isEmpty else { throw Failure("Unknown option or missing value: \(flag)") }
            let value = args.removeFirst()
            switch flag {
            case "--model": model = value
            case "--prompt": prompt = value
            case "--temperature":
                guard let n = Double(value), n.isFinite, (0...2).contains(n) else {
                    throw Failure("--temperature requires a number from 0 to 2.")
                }
                temperature = n
            case "--threads":
                guard let n = Int(value), (1...256).contains(n) else {
                    throw Failure("--threads requires an integer from 1 to 256.")
                }
                threads = n
            case "--port":
                guard let n = Int(value), (1024...65535).contains(n), command == "serve" else {
                    throw Failure("--port requires 1024...65535 and serve.")
                }
                port = n
            case "--context":
                guard let n = Int(value) else { throw Failure("--context requires an integer.") }
                context = n
            default:
                guard let n = Int(value) else { throw Failure("--max-tokens requires an integer.") }
                maxTokens = n
            }
        }
        if command == "run" && (prompt?.isEmpty != false) { throw Failure("run requires --prompt TEXT.") }
        if command != "run" && prompt != nil { throw Failure("--prompt is only accepted with run.") }
        if command == "serve" && maxTokens != nil {
            throw Failure("--max-tokens applies to chat/run. Provider clients set their own output budget.")
        }
    }
}

struct Model {
    let url: URL
    let bytes: UInt64
    let issue: String?
    var kvBytesPerToken: Double = 524288

    // Deliberately conservative, bounded-context heuristic. KV size varies by model.
    func estimatedBytes(context: Int) -> UInt64 {
        UInt64(Double(bytes) * 1.3 + gib + Double(context) * kvBytesPerToken)
    }
}

// Read only bounded metadata, never tensor data. Unknown/malformed metadata keeps
// the original conservative fallback. This is a memory estimate, not GGUF validation.
func estimatedKV(_ file: FileHandle) -> Double {
    do {
        try file.seek(toOffset: 0)
        let data = try file.read(upToCount: 32 * 1024 * 1024) ?? Data()
        var offset = 16
        func uint(_ size: Int) throws -> UInt64 {
            guard offset <= data.count - size else { throw Failure("Truncated GGUF metadata") }
            defer { offset += size }
            return (0..<size).reduce(0) { $0 | (UInt64(data[offset + $1]) << (8 * $1)) }
        }
        func string(_ retain: Bool = true) throws -> String {
            let n = try uint(8)
            guard n <= UInt64(data.count - offset) else { throw Failure("Oversized GGUF string") }
            defer { offset += Int(n) }
            return retain ? String(decoding: data[offset..<offset + Int(n)], as: UTF8.self) : ""
        }
        let count = try uint(8)
        guard count <= 10000 else { throw Failure("Too many GGUF metadata entries") }
        var numbers = [String: UInt64]()
        var architecture = ""
        func value(_ type: UInt64, retain: Bool, depth: Int = 0) throws -> UInt64? {
            let sizes: [UInt64: Int] = [0: 1, 1: 1, 2: 2, 3: 2, 4: 4, 5: 4, 6: 4, 7: 1, 10: 8, 11: 8, 12: 8]
            if let size = sizes[type] { return try uint(size) }
            if type == 8 { _ = try string(false); return nil }
            guard type == 9, depth == 0 else { throw Failure("Unsupported GGUF metadata type") }
            let subtype = try uint(4), length = try uint(8)
            guard length <= 1_000_000 else { throw Failure("Oversized GGUF array") }
            for _ in 0..<length { _ = try value(subtype, retain: false, depth: 1) }
            return nil
        }
        for _ in 0..<count {
            let key = try string(), type = try uint(4)
            if key == "general.architecture", type == 8 { architecture = try string() }
            else if let n = try value(type, retain: true), [4, 10].contains(type) { numbers[key] = n }
        }
        guard ["qwen2", "qwen3", "llama", "mistral"].contains(architecture) else { return 524288 }
        func number(_ key: String) -> Double? {
            guard let n = numbers[architecture + "." + key], (1...1_000_000).contains(n) else { return nil }
            return Double(n)
        }
        guard let layers = number("block_count"), let heads = number("attention.head_count"),
              let kvHeads = number("attention.head_count_kv"), let embedding = number("embedding_length") else { return 524288 }
        let key = number("attention.key_length") ?? embedding / heads
        let val = number("attention.value_length") ?? embedding / heads
        guard layers <= 256, heads <= 1024, kvHeads <= heads, embedding <= 65536,
              key <= 8192, val <= 8192 else { return 524288 }
        // Full FP16 K+V cache with 50% headroom and a 64 KiB/token floor.
        let estimate = layers * kvHeads * (key + val) * 2 * 1.5
        return estimate <= 16 * 1024 * 1024 ? max(65536, estimate) : 524288
    } catch { return 524288 }
}

func inspectModel(_ url: URL, root: URL) -> Model {
    var bytes: UInt64 = 0
    do {
        guard contained(url, in: root) else { throw Failure("symlink points outside the portable folder") }
        let values = try url.resourceValues(forKeys: [.isRegularFileKey, .fileSizeKey])
        guard values.isRegularFile == true else { throw Failure("not a regular file") }
        bytes = UInt64(values.fileSize ?? 0)
        guard url.lastPathComponent.range(of: #"-\d{5}-of-\d{5}\.gguf$"#,
                                         options: .regularExpression) == nil else {
            throw Failure("split GGUF is not supported in Phase 1")
        }
        let file = try FileHandle(forReadingFrom: url)
        defer { try? file.close() }
        let header = try file.read(upToCount: 24) ?? Data()
        guard header.count == 24, Array(header.prefix(4)) == [71, 71, 85, 70] else {
            throw Failure("not a GGUF model or incomplete header")
        }
        func uint(_ start: Int, _ count: Int) -> UInt64 {
            (0..<count).reduce(0) { $0 | (UInt64(header[start + $1]) << (8 * $1)) }
        }
        guard [2, 3].contains(uint(4, 4)), uint(8, 8) > 0, uint(16, 8) > 0,
              bytes > 1024 else { throw Failure("invalid or empty GGUF header") }
        return Model(url: url, bytes: bytes, issue: nil, kvBytesPerToken: estimatedKV(file))
    } catch { return Model(url: url, bytes: bytes, issue: String(describing: error)) }
}

func modelDirectory(_ root: URL, settings: Settings) throws -> URL {
    let shared = settings.models_directory == "../Models"
    let parent = shared ? root.deletingLastPathComponent() : root
    let directory = parent.appendingPathComponent(shared ? "Models" : "models")
    guard contained(directory, in: parent) else { throw Failure("models directory must stay inside the portable collection.") }
    return directory
}

func models(_ root: URL, settings: Settings) throws -> [Model] {
    let directory = try modelDirectory(root, settings: settings)
    return try fm.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.fileSizeKey])
        .filter { $0.pathExtension.lowercased() == "gguf" && !$0.lastPathComponent.hasPrefix("._") }
        .map { inspectModel($0, root: directory) }
        .sorted { $0.bytes == $1.bytes ? $0.url.lastPathComponent < $1.url.lastPathComponent : $0.bytes < $1.bytes }
}

func sha256(_ url: URL) throws -> String {
    let handle = try FileHandle(forReadingFrom: url)
    defer { try? handle.close() }
    var hash = SHA256()
    while let chunk = try handle.read(upToCount: 1 << 20), !chunk.isEmpty { hash.update(data: chunk) }
    return hash.finalize().map { String(format: "%02x", $0) }.joined()
}

func verify(_ root: URL, settings: Settings) throws {
    let manifest = root.appendingPathComponent("manifest.json")
    let hashes = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: manifest))
    guard !hashes.isEmpty else { throw Failure("Empty bundle manifest.") }
    for path in hashes.keys.sorted() {
        let sharedModel = path.hasPrefix("models/")
        let parent = sharedModel ? try modelDirectory(root, settings: settings) : root
        let url = parent.appendingPathComponent(sharedModel ? String(path.dropFirst(7)) : path)
        guard !path.hasPrefix("/"), contained(url, in: parent),
              try sha256(url) == hashes[path] else { throw Failure("Integrity check failed: \(path)") }
        print("OK  \(path)")
    }
    print("Verified \(hashes.count) packaged files. This detects changes; it is not a digital signature.")
}

// This is the only backend-specific argument mapping. No speculative runtime API.
func runtimeArguments(model: Model, settings: Settings, hardware: Hardware, options: Options) -> [String] {
    var args = ["--model", model.url.path, "--offline", "--conversation", "--jinja",
                "--ctx-size", String(settings.context), "--predict", String(settings.max_tokens),
                "--batch-size", "256", "--ubatch-size", "128", "--parallel", "1",
                "--threads", String(options.threads ?? max(1, hardware.cpuThreads - 2)),
                "--no-context-shift", "--fit", "off",
                "--temp", String(settings.temperature), "--seed", "42",
                "--no-escape", "--log-colors", "off",
                "--verbosity", options.verbose ? "5" : "1"]
    args += ["--system-prompt", "You are a local AI assistant running through Reflect Go (Reflect on the Go). Your model file is stored in the portable model folder. Inference runs on this Mac, using its CPU or GPU and memory. This chat has no internet connection, web browsing, or cloud inference. Do not claim to run in a cloud service. Your original model developer is separate from this local deployment."]
    if options.cpu || hardware.metalDevice == nil {
        args += ["--device", "none", "--gpu-layers", "0", "--no-kv-offload", "--no-op-offload"]
    } else { args += ["--gpu-layers", "all"] }
    if options.verbose { args += ["--perf"] }
    if let prompt = options.prompt {
        args += ["--single-turn", "--prompt", prompt, "--simple-io", "--color", "off", "--no-display-prompt"]
    }
    return args
}

func providerArguments(_ root: URL, model: Model, settings: Settings, hardware: Hardware, options: Options) throws -> [String] {
    let state = root.appendingPathComponent("state")
    let lock = state.appendingPathComponent("provider.lock")
    let fd = open(lock.path, O_CREAT | O_RDWR | O_NOFOLLOW, 0o600)
    guard fd >= 0, flock(fd, LOCK_EX | LOCK_NB) == 0 else {
        if fd >= 0 { close(fd) }
        throw Failure("A provider is already running from this folder, or its lock is unavailable.")
    }
    // Keep the lock across exec; it is released automatically on process exit.
    _ = fcntl(fd, F_SETFD, 0)
    let socketFD = socket(AF_INET, SOCK_STREAM, 0)
    guard socketFD >= 0 else { throw Failure("Cannot check provider port.") }
    // Match the server's listener behavior: a recently closed connection in
    // TIME_WAIT must not prevent immediate restart on an otherwise free port.
    var reuse: Int32 = 1
    guard setsockopt(socketFD, SOL_SOCKET, SO_REUSEADDR, &reuse, socklen_t(MemoryLayout<Int32>.size)) == 0 else {
        close(socketFD)
        throw Failure("Cannot configure provider port check.")
    }
    var address = sockaddr_in()
    address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
    address.sin_family = sa_family_t(AF_INET)
    address.sin_port = in_port_t(options.port).bigEndian
    address.sin_addr.s_addr = inet_addr("127.0.0.1")
    let available = withUnsafePointer(to: &address) { pointer in
        pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
            bind(socketFD, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) == 0
        }
    }
    close(socketFD)
    guard available else { throw Failure("Port \(options.port) is unavailable. Stop its provider or use --port with a different port.") }
    let keyURL = state.appendingPathComponent("provider-key")
    guard contained(keyURL, in: state) else { throw Failure("Provider key must stay on the drive.") }
    if !fm.fileExists(atPath: keyURL.path) {
        var random = [UInt8](repeating: 0, count: 32)
        guard SecRandomCopyBytes(kSecRandomDefault, random.count, &random) == errSecSuccess else {
            throw Failure("Cannot generate provider key.")
        }
        let key = random.map { String(format: "%02x", $0) }.joined()
        try Data((key + "\n").utf8).write(to: keyURL, options: .atomic)
        try fm.setAttributes([.posixPermissions: 0o600], ofItemAtPath: keyURL.path)
    }
    let key = try String(contentsOf: keyURL).trimmingCharacters(in: .whitespacesAndNewlines)
    guard key.count == 64, key.allSatisfy({ $0.isHexDigit && $0.isASCII }) else {
        throw Failure("Invalid provider key. Move state/provider-key aside to generate a new one.")
    }
    let base = "http://127.0.0.1:\(options.port)"
    let connection: [String: Any] = ["base_url": base, "model": "reflect-go-local", "context": settings.context,
                                    "model_file": model.url.lastPathComponent]
    let connectionURL = state.appendingPathComponent("provider.json")
    guard contained(connectionURL, in: state) else { throw Failure("Provider connection settings must stay on the drive.") }
    try JSONSerialization.data(withJSONObject: connection, options: [.prettyPrinted, .sortedKeys])
        .write(to: connectionURL, options: .atomic)
    note("Reflect Go local provider · \(base)/v1 · model: reflect-go-local")
    note("Wait for 'listening' before connecting. This terminal reports tokens/second after each response.")
    note("Use Connect Codex.command or Connect Claude.command in another terminal. Ctrl-C stops the provider; stop it before ejecting.")
    var args = ["--model", model.url.path, "--alias", "reflect-go-local", "--offline", "--jinja",
                "--host", "127.0.0.1", "--port", String(options.port), "--api-key-file", keyURL.path,
                "--ctx-size", String(settings.context), "--parallel", "1",
                "--batch-size", "256", "--ubatch-size", "128",
                "--threads", String(options.threads ?? max(1, hardware.cpuThreads - 2)), "--fit", "off",
                "--temp", String(settings.temperature), "--seed", "42", "--reasoning", "off",
                "--no-webui", "--no-ui-mcp-proxy", "--no-agent", "--metrics", "--no-slots",
                "--cors-origins", base, "--no-cors-credentials",
                "--cache-ram", "0", "--ctx-checkpoints", "0", "--verbosity", options.verbose ? "5" : "3"]
    if options.cpu || hardware.metalDevice == nil {
        args += ["--device", "none", "--gpu-layers", "0", "--no-kv-offload", "--no-op-offload"]
    } else { args += ["--gpu-layers", "all"] }
    return args
}

func launch(_ root: URL, model: Model, hardware: Hardware, settings: Settings, options: Options) throws -> Never {
    let serving = options.command == "serve"
    let runtime = root.appendingPathComponent("runtime/macos-arm64/" + (serving ? "llama-server" : "llama-completion"))
    guard contained(runtime, in: root), fm.isExecutableFile(atPath: runtime.path) else {
        throw Failure("Bundled runtime is missing or not executable. Restore the complete PortableAI folder.")
    }
    let state = root.appendingPathComponent("state")
    for relative in ["state", "state/home", "state/cache", "state/tmp", "state/config"] {
        let url = root.appendingPathComponent(relative)
        guard contained(url, in: root) else { throw Failure("\(relative) must stay inside PortableAI.") }
        try fm.createDirectory(at: url, withIntermediateDirectories: true,
                               attributes: [.posixPermissions: 0o700])
    }
    guard fm.isWritableFile(atPath: state.path) else { throw Failure("PortableAI needs a writable state directory on the drive.") }
    // Explicit environment: ignore inherited LLAMA_ARG_*, HF_TOKEN, DYLD_* and remote RPC settings.
    var env = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
               "HOME": state.appendingPathComponent("home").path,
               "CFFIXED_USER_HOME": state.appendingPathComponent("home").path,
               "XDG_CACHE_HOME": state.appendingPathComponent("cache").path,
               "XDG_CONFIG_HOME": state.appendingPathComponent("config").path,
               "LLAMA_CACHE": state.appendingPathComponent("cache").path,
               "TMPDIR": state.appendingPathComponent("tmp").path + "/",
               "HF_HUB_OFFLINE": "1"]
    for key in ["TERM", "LANG", "LC_ALL"] {
        if let value = ProcessInfo.processInfo.environment[key] { env[key] = value }
    }
    guard chdir(state.path) == 0 else { throw Failure("Cannot enter the drive's state directory.") }
    let arguments = serving ? try providerArguments(root, model: model, settings: settings, hardware: hardware, options: options)
                            : runtimeArguments(model: model, settings: settings, hardware: hardware, options: options)
    let args = [runtime.path] + arguments
    var argv = args.map { strdup($0) } + [nil]
    var envp = env.keys.sorted().map { strdup("\($0)=\(env[$0]!)") } + [nil]
    fflush(nil)
    execve(runtime.path, &argv, &envp)
    let message = String(cString: strerror(errno))
    for pointer in argv + envp { free(pointer) }
    throw Failure("Runtime could not start: \(message). Check volume execution permissions and macOS security settings.")
}

func main() throws {
    if CommandLine.arguments.contains("--help") {
        print("""
        Reflect Go — Reflect on the Go (macOS 13.3+ / Apple Silicon)
        Usage: portableai [chat|run|serve|doctor|inventory|models|verify] [options]
          chat                    Choose a local model and chat (default)
          run --prompt TEXT       Generate one reply and exit
          serve                   Start the authenticated local model provider
          --port N                Loopback port for serve (default 8765)
          doctor                  Print detected hardware as JSON
          inventory               Print hardware and model estimates as JSON
          models                  List local GGUF files and estimated fit
          verify                  Verify packaged runtime and model checksums
          --model FILENAME        Select a file directly inside models/
          --cpu                   Use CPU inference
          --context N             Override context (512...32768)
          --temperature N         Sampling temperature (0...2)
          --threads N             CPU threads (1...host CPU thread count)
          --max-tokens N          Limit chat/run generated tokens per reply
          --verbose               Show runtime loading/performance diagnostics
        Settings: config/portableai.json. No runtime downloads. Provider binds only to 127.0.0.1.
        In chat: Ctrl-D on an empty input line quits; Ctrl-C interrupts, twice exits.
        Quit before ejecting the drive. Full cross-Mac portability still needs testing.
        """)
        return
    }
    let options = try Options(Array(CommandLine.arguments.dropFirst()))
    let root = try rootDirectory()
    let version = ProcessInfo.processInfo.operatingSystemVersion
    guard version.majorVersion > 13 || (version.majorVersion == 13 && version.minorVersion >= 3) else {
        throw Failure("This runtime requires macOS 13.3 or later on Apple Silicon.")
    }
    let hardware = Hardware()
    guard hardware.physicalMemoryBytes > 0 else { throw Failure("Cannot read physical memory; hardware inspection may be restricted.") }
    if options.command == "doctor" {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        print(String(decoding: try encoder.encode(hardware), as: UTF8.self))
        return
    }
    let config = root.appendingPathComponent("config/portableai.json")
    guard contained(config, in: root) else { throw Failure("Configuration must stay inside PortableAI.") }
    var settings: Settings
    do { settings = try JSONDecoder().decode(Settings.self, from: Data(contentsOf: config)) }
    catch { throw Failure("Cannot read config/portableai.json: \(error)") }
    if ["serve", "inventory"].contains(options.command) { settings.context = settings.provider_context ?? 16384 }
    if let context = options.context { settings.context = context }
    if let tokens = options.maxTokens { settings.max_tokens = tokens }
    if let temperature = options.temperature { settings.temperature = temperature }
    if let threads = options.threads, threads > hardware.cpuThreads {
        throw Failure("--threads exceeds this computer's CPU thread count (\(hardware.cpuThreads)).")
    }
    try settings.validate()
    if options.command == "verify" { try verify(root, settings: settings); return }
    let candidates = try models(root, settings: settings)
    if options.command == "inventory" {
        struct Entry: Encodable {
            let name: String
            let bytes: UInt64
            let estimatedBytes: UInt64
            let eligible: Bool
            let issue: String?
        }
        struct Inventory: Encodable {
            let hardware: Hardware
            let context: Int
            let defaultModel: String?
            let modelsPath: String
            let models: [Entry]
        }
        let entries = candidates.map { model in
            Entry(name: model.url.lastPathComponent, bytes: model.bytes,
                  estimatedBytes: model.estimatedBytes(context: settings.context),
                  eligible: model.issue == nil && model.estimatedBytes(context: settings.context) <= hardware.screeningBudgetBytes,
                  issue: model.issue)
        }
        let snapshot = Inventory(hardware: hardware, context: settings.context,
                                 defaultModel: settings.provider_model,
                                 modelsPath: try modelDirectory(root, settings: settings).path, models: entries)
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        print(String(decoding: try encoder.encode(snapshot), as: UTF8.self))
        return
    }
    guard !candidates.isEmpty else { throw Failure("No models found. Copy a compatible single-file text GGUF into models/.") }
    note("\(hardware.cpu) · \(String(format: "%.1f", Double(hardware.physicalMemoryBytes) / gib)) GiB RAM · \(hardware.metalDevice ?? "no Metal device; CPU fallback")")
    note("Memory screen: \(String(format: "%.1f", Double(hardware.screeningBudgetBytes) / gib)) GiB budget. Estimates are not free-memory measurements or fit guarantees.")
    func eligible(_ model: Model) -> Bool {
        model.issue == nil && model.estimatedBytes(context: settings.context) <= hardware.screeningBudgetBytes
    }
    for (index, model) in candidates.enumerated() {
        let status = model.issue ?? (eligible(model) ? "eligible" : "too large for conservative budget")
        note("\(index + 1). \(model.url.lastPathComponent) — \(String(format: "%.2f", Double(model.bytes) / gib)) GiB file, ~\(String(format: "%.2f", Double(model.estimatedBytes(context: settings.context)) / gib)) GiB estimated — \(status)")
    }
    if options.command == "models" { return }
    let selected: Model
    if let name = options.model ?? (options.command == "serve" ? settings.provider_model : nil) {
        guard !name.contains("/"), let found = candidates.first(where: { $0.url.lastPathComponent == name }) else {
            throw Failure("Model not found in models/: \(name)")
        }
        selected = found
    } else {
        guard let recommended = candidates.firstIndex(where: eligible) else {
            throw Failure("No model passes the memory/format screen. Use a smaller compatible model or close other apps and reassess.")
        }
        if options.command == "chat" && isatty(STDIN_FILENO) != 0 {
            note("Choose a model [\(recommended + 1), smallest eligible model]:")
            guard let line = readLine() else { return }
            let answer = line.trimmingCharacters(in: .whitespacesAndNewlines)
            let index = answer.isEmpty ? recommended + 1 : Int(answer) ?? 0
            guard (1...candidates.count).contains(index) else { throw Failure("Invalid model selection.") }
            selected = candidates[index - 1]
        } else { selected = candidates[recommended] }
    }
    guard selected.issue == nil else { throw Failure(selected.issue!) }
    guard eligible(selected) else { throw Failure("Selected model exceeds the conservative memory budget. Choose a smaller model or context.") }
    let manifestURL = root.appendingPathComponent("manifest.json")
    let hashes = try JSONDecoder().decode([String: String].self, from: Data(contentsOf: manifestURL))
    if let expected = hashes["models/" + selected.url.lastPathComponent] {
        guard try sha256(selected.url) == expected else {
            throw Failure("Model checksum mismatch. Restore the packaged model or rebuild the bundle.")
        }
    }
    note("Starting \(selected.url.lastPathComponent) with \(options.cpu || hardware.metalDevice == nil ? "CPU" : "Metal"), context \(settings.context).")
    if options.command == "chat" { note("Ctrl-D at an empty prompt quits. Avoid submitting a blank message; the original runtime treats it as a request to continue.") }
    try launch(root, model: selected, hardware: hardware, settings: settings, options: options)
}

do { try main() }
catch { note("PortableAI: \(error)"); exit(1) }
