private var phaseLabel: String {
    let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "0.3.1"
    return version.hasPrefix("0.") ? String(version.dropFirst(2)) : version
}

import AppKit
import SwiftUI
import Darwin

private let fileManager = FileManager.default
private let accent = Color(red: 0.65, green: 0.90, blue: 0.66)
private let surface = Color(red: 0.105, green: 0.12, blue: 0.115)
private let canvasBackground = Color(red: 0.065, green: 0.078, blue: 0.074)
private func giB(_ bytes: UInt64) -> String { String(format: "%.1f GiB", Double(bytes) / 1073741824) }
private func shellQuote(_ text: String) -> String { "'" + text.replacingOccurrences(of: "'", with: "'\\''") + "'" }
private struct AppError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}
struct Machine: Decodable {
    let cpu: String
    let os: String
    let physicalMemoryBytes: UInt64
    let cpuThreads: Int
    let metalDevice: String?
    let screeningBudgetBytes: UInt64
}
struct LocalModel: Decodable, Identifiable {
    let name: String
    let bytes: UInt64
    let estimatedBytes: UInt64
    let eligible: Bool
    let issue: String?
    var id: String { name }
    var title: String { name.replacingOccurrences(of: ".gguf", with: "", options: .caseInsensitive) }
}
struct Inventory: Decodable {
    let hardware: Machine
    let context: Int
    let defaultModel: String?
    let modelsPath: String
    let models: [LocalModel]
}
struct Preferences: Codable {
    var model = ""
    var context = 16384
    var cpuOnly = false
    var temperature = 0.2
    var threads = 0
    var port = 8766
}

// Network requests never follow redirects or inherit proxy settings. The bearer
// key is used only for this app's own loopback process and is never displayed.
final class LocalSessionDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) { completionHandler(nil) }
}

@MainActor final class Workstation: ObservableObject {
    let root: URL
    @Published var inventory: Inventory?
    @Published var preferences = Preferences()
    @Published var page = "Workstation"
    @Published var status = "Engine stopped"
    @Published var detail = "Choose a model, then start your portable inference engine."
    @Published var error = ""
    @Published var notice = ""
    @Published var loadingInventory = false
    @Published var active = false
    @Published var ready = false
    @Published var stopping = false
    @Published var testing = false
    @Published var verifying = false
    @Published var response = ""
    @Published var log = "No engine session yet."
    @Published var verification = ""
    @Published var speed: Double?
    @Published var generated = 0
    @Published var requests = 0
    @Published var residentBytes: UInt64?
    @Published var systemUsedBytes: UInt64?
    @Published var startSeconds: Double?
    @Published var project: URL?
    @Published var codexInstalled = false
    @Published var claudeInstalled = false
    @Published var opencodeInstalled = false
    private var engine: Process?
    private var ownership: Pipe?
    private var enginePID: Int32?
    private var sessionLock: Int32 = -1
    private var logHandle: FileHandle?
    private var timer: Timer?
    private var sessionID = UUID()
    private var started = Date()
    private var polling = false
    private var probeTask: Task<Void, Never>?
    private var pendingQuit = false
    private var inventoryRevision = 0
    private var serverAnnounced = false
    private var failedProbes = 0
    private var readingLog = false
    private var stopReason: String?
    private let networkDelegate = LocalSessionDelegate()
    private lazy var network: URLSession = {
        let config = URLSessionConfiguration.ephemeral
        config.connectionProxyDictionary = [:]
        config.timeoutIntervalForRequest = 4
        config.timeoutIntervalForResource = 120
        config.urlCache = nil
        return URLSession(configuration: config, delegate: networkDelegate, delegateQueue: nil)
    }()
    var selected: LocalModel? { inventory?.models.first { $0.name == preferences.model } }
    var baseURL: String { "http://127.0.0.1:\(preferences.port)" }
    var canStart: Bool { !active && !loadingInventory && !verifying && selected?.eligible == true }
    var state: URL { root.appendingPathComponent("state") }

    init(root: URL) {
        self.root = root
        // A removable-volume access check can block open(). Draw the window
        // before touching saved settings, and keep that I/O off the main actor.
        loadingInventory = true
        detail = "Reading the drive. If macOS asks, allow access to your Reflect Go drive."
        Task { [weak self] in
            let data = await Task.detached {
                try? Data(contentsOf: root.appendingPathComponent("state/workstation.json"))
            }.value
            guard let self else { return }
            if let data, let saved = try? JSONDecoder().decode(Preferences.self, from: data),
               [2048,4096,8192,16384,32768].contains(saved.context), (1024...65535).contains(saved.port),
               saved.temperature.isFinite, (0...2).contains(saved.temperature), (0...256).contains(saved.threads) {
                self.preferences = saved
            } else if data != nil {
                self.notice = "Saved run settings could not be read. Defaults have been restored; review them before starting."
            }
            self.codexInstalled = self.clientExists("codex")
            self.claudeInstalled = self.clientExists("claude")
            self.opencodeInstalled = self.clientExists("opencode")
            self.detail = "Choose a model, then start your portable inference engine."
            self.refresh()
            self.readLog()
        }
        timer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.tick() }
        }
    }
    func safeState(_ name: String) throws -> URL {
        let url = state.appendingPathComponent(name)
        let base = root.resolvingSymlinksInPath().path + "/"
        guard state.resolvingSymlinksInPath().path.hasPrefix(base),
              url.resolvingSymlinksInPath().path.hasPrefix(state.resolvingSymlinksInPath().path + "/") else {
            throw AppError(message: "The state folder must stay inside this phase folder.")
        }
        try fileManager.createDirectory(at: state, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        return url
    }
    func executable(_ relative: String) throws -> URL {
        let url = root.appendingPathComponent(relative)
        guard url.resolvingSymlinksInPath().path.hasPrefix(root.resolvingSymlinksInPath().path + "/"),
              fileManager.isExecutableFile(atPath: url.path) else {
            throw AppError(message: "Missing portable software: \(relative). Restore the complete phase folder.")
        }
        return url
    }
    func engineHost() throws -> URL {
        try executable("Reflect Go.app/Contents/MacOS/engine-host")
    }
    func portableEngine() throws -> URL {
        #if arch(arm64)
        return try executable("bin/macos-arm64/reflectgo")
        #else
        return try executable("bin/macos-x64/reflectgo")
        #endif
    }
    func persist() {
        do { try JSONEncoder().encode(preferences).write(to: safeState("workstation.json"), options: .atomic) }
        catch { self.error = error.localizedDescription }
    }
    func refresh() {
        guard !active else { return }
        loadingInventory = true
        inventoryRevision += 1
        let revision = inventoryRevision
        let context = preferences.context
        Task {
            do {
                let result = try await runUtility(["inventory", "--context", String(context)])
                let value = try JSONDecoder().decode(Inventory.self, from: result)
                guard revision == inventoryRevision else { return }
                inventory = value
                if !value.models.contains(where: { $0.name == preferences.model }) {
                    if !preferences.model.isEmpty {
                        notice = "The previous model is no longer in Models. Review the new selection before starting."
                    }
                    preferences.model = value.models.first(where: { $0.name == value.defaultModel && $0.eligible })?.name
                        ?? value.models.first(where: { $0.eligible })?.name ?? value.models.first?.name ?? ""
                }
                if preferences.threads > value.hardware.cpuThreads { preferences.threads = 0 }
                error = ""
                persist()
            } catch {
                guard revision == inventoryRevision else { return }
                self.error = error.localizedDescription
            }
            guard revision == inventoryRevision else { return }
            loadingInventory = false
            codexInstalled = clientExists("codex")
            claudeInstalled = clientExists("claude")
            opencodeInstalled = clientExists("opencode")
            sampleMemory()
        }
    }
    // Utilities run away from the UI thread; merged output is drained while the
    // process runs, so even a large integrity report cannot fill a pipe and hang.
    func runUtility(_ arguments: [String]) async throws -> Data {
        let binary = try portableEngine()
        let host = try engineHost()
        return try await Task.detached {
            let child = Process(), pipe = Pipe()
            let limit: Double = arguments == ["verify"] ? 600 : (arguments.first == "inventory" ? 150 : 30)
            child.executableURL = host
            child.arguments = ["--timeout", String(limit), binary.path] + arguments
            child.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "en_US.UTF-8"]
            child.standardOutput = pipe
            child.standardError = pipe
            let owner = Pipe()
            child.standardInput = owner
            defer { try? owner.fileHandleForWriting.close() }
            try child.run()
            // The OS can delay the helper in its loader, before its own timeout
            // starts. Keep an independent deadline in the app as well.
            let began = Date()
            let deadline = DispatchWorkItem { if child.isRunning { child.terminate() } }
            let hardDeadline = DispatchWorkItem {
                if child.isRunning { kill(child.processIdentifier, SIGKILL) }
            }
            DispatchQueue.global().asyncAfter(deadline: .now() + limit, execute: deadline)
            // The helper normally gives its child eight seconds to stop. A
            // helper blocked by the OS before main cannot run that cleanup.
            DispatchQueue.global().asyncAfter(deadline: .now() + limit + 12, execute: hardDeadline)
            defer { deadline.cancel(); hardDeadline.cancel() }
            try? owner.fileHandleForReading.close()
            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            child.waitUntilExit()
            if child.terminationStatus == 124 || Date().timeIntervalSince(began) >= limit {
                throw AppError(message: "The drive check timed out. Check that the drive is connected and macOS has allowed access, then refresh.")
            }
            if child.terminationStatus != 0 {
                throw AppError(message: String(decoding: data.suffix(8000), as: UTF8.self))
            }
            return data
        }.value
    }
    func start() {
        guard canStart else { return }
        guard (1024...65535).contains(preferences.port) else { error = "Choose a port from 1024 to 65535."; return }
        do {
            let child = Process()
            child.executableURL = try engineHost()
            var args = [try portableEngine().path, "serve", "--model", preferences.model, "--context", String(preferences.context),
                        "--port", String(preferences.port), "--temperature", String(preferences.temperature)]
            if preferences.cpuOnly { args.append("--cpu") }
            if preferences.threads > 0 { args += ["--threads", String(preferences.threads)] }
            child.arguments = args
            child.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "en_US.UTF-8"]
            let owner = Pipe()
            child.standardInput = owner
            // Lock before rotating logs, so another app cannot erase a live session.
            let lockURL = try safeState("workstation.lock")
            let lock = open(lockURL.path, O_CREAT | O_RDWR | O_NOFOLLOW | O_CLOEXEC, 0o600)
            guard lock >= 0 else { throw AppError(message: "Cannot open the workstation lock on this drive.") }
            guard flock(lock, LOCK_EX | LOCK_NB) == 0 else {
                close(lock)
                throw AppError(message: "This phase already has an engine open in another app window. Stop it there first.")
            }
            sessionLock = lock
            let logURL = try safeState("workstation-engine.log")
            if fileManager.fileExists(atPath: logURL.path) {
                let previous = try safeState("workstation-previous.log")
                if fileManager.fileExists(atPath: previous.path) { try fileManager.removeItem(at: previous) }
                try fileManager.moveItem(at: logURL, to: previous)
            }
            guard fileManager.createFile(atPath: logURL.path, contents: nil, attributes: [.posixPermissions: 0o600]) else {
                throw AppError(message: "The drive is not writable. Check its connection and free space.")
            }
            logHandle = try FileHandle(forWritingTo: logURL)
            child.standardOutput = logHandle
            child.standardError = logHandle
            let id = UUID()
            sessionID = id
            child.terminationHandler = { [weak self] process in
                let code = process.terminationStatus
                Task { @MainActor in self?.didExit(id: id, code: code) }
            }
            ready = false; stopping = false; speed = nil; generated = 0; requests = 0
            response = ""; error = ""; startSeconds = nil
            started = Date()
            serverAnnounced = false; failedProbes = 0; enginePID = nil; stopReason = nil
            log = "Starting engine…"
            try child.run()
            try? owner.fileHandleForReading.close()
            ownership = owner
            engine = child
            active = true
            status = "Loading model"
            detail = "Checking the model and loading it into this Mac’s memory."
            persist()
        } catch {
            releaseOwnership()
            try? logHandle?.close(); logHandle = nil
            self.error = error.localizedDescription
            status = "Couldn’t start"
        }
    }
    func stop() {
        guard let child = engine, child.isRunning, !stopping else { return }
        stopping = true; ready = false; status = "Stopping engine"
        detail = "Finishing shutdown and releasing the model’s memory."
        probeTask?.cancel()
        // EOF also arrives automatically if the app crashes or is force-quit.
        try? ownership?.fileHandleForWriting.close()
        ownership = nil
    }
    private func releaseOwnership() {
        try? ownership?.fileHandleForWriting.close()
        ownership = nil
        if sessionLock >= 0 { close(sessionLock); sessionLock = -1 }
    }
    func confirmStop() {
        guard active else { return }
        let alert = NSAlert()
        alert.messageText = "Stop the portable inference engine?"
        alert.informativeText = "Connected clients will lose local inference. Finish or cancel their tasks first. The model stays on your drive."
        alert.addButton(withTitle: "Stop engine")
        alert.addButton(withTitle: "Keep running")
        if alert.runModal() == .alertFirstButtonReturn { stop() }
    }
    func requestQuit() -> NSApplication.TerminateReply {
        if !loadingInventory { persist() }
        guard active else { return .terminateNow }
        let alert = NSAlert()
        alert.messageText = "Stop the engine and quit Reflect Go?"
        alert.informativeText = "Quit your connected clients first. Reflect Go will unload the model before closing."
        alert.addButton(withTitle: "Stop and quit")
        alert.addButton(withTitle: "Cancel")
        guard alert.runModal() == .alertFirstButtonReturn else { return .terminateCancel }
        pendingQuit = true
        stop()
        return .terminateLater
    }
    private func didExit(id: UUID, code: Int32) {
        guard id == sessionID else { return }
        let requested = stopping
        readLog()
        active = false; ready = false; stopping = false; testing = false; residentBytes = nil; requests = 0
        probeTask?.cancel()
        engine = nil
        enginePID = nil
        releaseOwnership()
        try? logHandle?.close(); logHandle = nil
        status = requested ? "Engine stopped" : (startSeconds == nil ? "Couldn’t start" : "Engine exited")
        detail = requested ? "Model unloaded. Quit connected clients before ejecting the drive." : "Open Diagnostics for the engine’s output."
        if let stopReason { status = "Startup timed out"; error = stopReason }
        if !requested {
            let lines = log.components(separatedBy: .newlines).filter { !$0.isEmpty }
            let reason = lines.last(where: { $0.contains("PortableAI:") || $0.contains("Reflect Go:") }) ?? "The engine exited (code \(code)). See Diagnostics."
            if reason.lowercased().contains("port ") && reason.contains("is unavailable") {
                error = "Local port \(preferences.port) is already in use. Choose another under Run settings → Advanced, or stop the other engine."
            } else if reason.contains("provider is already running") {
                error = "An engine is already running from this phase folder. Stop it in its original window, then try again."
            } else {
                error = reason.replacingOccurrences(of: "PortableAI: ", with: "").replacingOccurrences(of: "Reflect Go: ", with: "")
            }
        }
        if pendingQuit { NSApp.reply(toApplicationShouldTerminate: true) }
    }
    func request(_ path: String, timeout: Double = 4) throws -> URLRequest {
        guard active, let child = engine, child.isRunning else { throw AppError(message: "Start the engine first.") }
        let key = try String(contentsOf: safeState("provider-key")).trimmingCharacters(in: .whitespacesAndNewlines)
        guard key.count == 64, key.allSatisfy({ $0.isASCII && $0.isHexDigit }) else { throw AppError(message: "Invalid local provider key.") }
        var request = URLRequest(url: URL(string: baseURL + path)!, timeoutInterval: timeout)
        request.setValue("Bearer " + key, forHTTPHeaderField: "Authorization")
        return request
    }
    func tick() async {
        sampleMemory()
        guard active, !stopping, !polling else { return }
        polling = true
        defer { polling = false }
        let id = sessionID
        readLog()
        // Cold model reads from a portable drive can exceed five minutes.
        if startSeconds == nil && Date().timeIntervalSince(started) >= 600 {
            stopReason = "The engine did not become ready within ten minutes and was stopped. Check Diagnostics and file integrity, then retry with a smaller model or context."
            stop()
            return
        }
        // Wait for this session's own startup log before probing. This avoids
        // mistaking another engine on a busy port for our newly loading child.
        if startSeconds == nil && !serverAnnounced {
            detail = "Checking and loading weights · \(Int(Date().timeIntervalSince(started))) seconds"
            return
        }
        do {
            let (data, reply) = try await network.data(for: request("/metrics"))
            guard id == sessionID, active, !stopping else { return }
            guard (reply as? HTTPURLResponse)?.statusCode == 200 else { throw AppError(message: "Provider is loading or unavailable.") }
            var metrics: [String: Double] = [:]
            for line in String(decoding: data, as: UTF8.self).split(separator: "\n") where !line.hasPrefix("#") {
                let values = line.split(separator: " ", omittingEmptySubsequences: true)
                if values.count == 2, let value = Double(values[1]), value.isFinite { metrics[String(values[0])] = value }
            }
            let tokens = metrics["llamacpp:tokens_predicted_total"] ?? 0
            generated = Int(max(0, min(tokens, Double(Int32.max))))
            // The gauge falls back to zero while idle. Cumulative generation
            // counters retain a real session average after the reply finishes.
            let generationSeconds = metrics["llamacpp:tokens_predicted_seconds_total"] ?? 0
            speed = generated > 0 && generationSeconds > 0 ? tokens / generationSeconds : nil
            requests = Int(max(0, min(metrics["llamacpp:requests_processing"] ?? 0, 10000)))
            if startSeconds == nil { startSeconds = Date().timeIntervalSince(started) }
            ready = true
            failedProbes = 0
            status = requests > 0 ? "Engine working" : "Engine ready"
            detail = requests > 0 ? "Your local model is processing a request." : "Connect a client or try a quick response below."
        } catch {
            guard id == sessionID, active, !stopping else { return }
            ready = false
            failedProbes += 1
            status = startSeconds == nil ? "Loading model" : "Checking connection"
            detail = startSeconds == nil ? "Checking and loading weights · \(Int(Date().timeIntervalSince(started))) seconds" : "The engine is running; waiting for a response from its local API."
            if startSeconds != nil && failedProbes >= 3 {
                status = "Connection unavailable"
                detail = "The engine is not answering local checks. Finish or cancel your client task, then Stop & unload and start again. See Diagnostics for the last output."
            }
        }
    }
    func sampleMemory() {
        var info = vm_statistics64()
        var count = mach_msg_type_number_t(MemoryLayout<vm_statistics64>.size / MemoryLayout<integer_t>.size)
        let result = withUnsafeMutablePointer(to: &info) { ptr in
            ptr.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                host_statistics64(mach_host_self(), HOST_VM_INFO64, $0, &count)
            }
        }
        if result == KERN_SUCCESS {
            let pages = UInt64(info.active_count) + UInt64(info.wire_count) + UInt64(info.compressor_page_count)
            systemUsedBytes = pages * UInt64(vm_kernel_page_size)
        }
        if let child = engine, child.isRunning, let enginePID {
            var usage = rusage_info_v2()
            let code = withUnsafeMutablePointer(to: &usage) { ptr in
                ptr.withMemoryRebound(to: rusage_info_t?.self, capacity: 1) {
                    proc_pid_rusage(enginePID, RUSAGE_INFO_V2, $0)
                }
            }
            residentBytes = code == 0 ? usage.ri_resident_size : nil
        }
    }
    func readLog() {
        guard !readingLog, let url = try? safeState("workstation-engine.log") else { return }
        readingLog = true
        let id = sessionID
        Task {
            // macOS privacy prompts and removable media can block open(). Never
            // let a diagnostics read freeze the window or its Quit command.
            let result = await Task.detached { () -> (String, String)? in
                guard let handle = try? FileHandle(forReadingFrom: url) else { return nil }
                defer { try? handle.close() }
                let prefix = (try? handle.read(upToCount: 4096)) ?? Data()
                if let end = try? handle.seekToEnd() { try? handle.seek(toOffset: end > 48000 ? end - 48000 : 0) }
                let tail = (try? handle.read(upToCount: 48000)) ?? Data()
                return (String(decoding: prefix, as: UTF8.self), String(decoding: tail, as: UTF8.self))
            }.value
            readingLog = false
            guard id == sessionID, let (prefix, tail) = result else { return }
            if active && enginePID == nil {
                for line in prefix.components(separatedBy: .newlines) {
                    if line.hasPrefix("Reflect Go engine PID: ") {
                        enginePID = Int32(line.dropFirst("Reflect Go engine PID: ".count))
                    }
                }
            }
            log = tail
                .replacingOccurrences(of: #"\b[0-9a-fA-F]{64}\b"#, with: "[redacted]", options: .regularExpression)
            if active && log.contains("llama_server: listening on " + baseURL) { serverAnnounced = true }
        }
    }
    func quickTest() {
        guard ready, !testing, requests == 0 else { return }
        testing = true; response = "Generating a short response…"
        let id = sessionID
        probeTask = Task {
            defer { if id == sessionID { testing = false } }
            do {
                var probe = try request("/v1/chat/completions", timeout: 90)
                probe.httpMethod = "POST"
                probe.setValue("application/json", forHTTPHeaderField: "Content-Type")
                probe.httpBody = try JSONSerialization.data(withJSONObject: [
                    "model": "reflect-go-local", "max_tokens": 64, "temperature": 0.2,
                    "messages": [["role": "system", "content": "You are running locally through Reflect Go on this Mac. Be brief."],
                                 ["role": "user", "content": "Write one friendly sentence welcoming me to my portable AI workstation."]]])
                let (data, reply) = try await network.data(for: probe)
                guard id == sessionID, active, !stopping else { return }
                guard (reply as? HTTPURLResponse)?.statusCode == 200,
                      let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let choices = object["choices"] as? [[String: Any]],
                      let message = choices.first?["message"] as? [String: Any],
                      let text = message["content"] as? String else { throw AppError(message: "The provider did not return a successful response. See Diagnostics.") }
                response = text.trimmingCharacters(in: .whitespacesAndNewlines)
                await tick()
            } catch {
                if id == sessionID { response = stopping || !active ? "Test stopped." : error.localizedDescription }
            }
        }
    }
    func verify() {
        guard !verifying, !active else { return }
        verifying = true; verification = "Checking packaged software and the bundled 4B model…"
        Task {
            do {
                let output = try await runUtility(["verify"])
                verification = String(decoding: output, as: UTF8.self)
            } catch { verification = error.localizedDescription }
            verifying = false
        }
    }
    func chooseProject() {
        let panel = NSOpenPanel()
        panel.title = "Choose the project your AI client will work on"
        panel.canChooseDirectories = true; panel.canChooseFiles = false; panel.allowsMultipleSelection = false
        panel.prompt = "Choose project"
        guard panel.runModal() == .OK, let url = panel.url else { return }
        let path = url.resolvingSymlinksInPath().path
        let collection = root.deletingLastPathComponent().resolvingSymlinksInPath().path
        guard path != collection, !path.hasPrefix(collection + "/") else {
            error = "Choose a project outside the Reflect Go collection so client edits cannot affect your models or engine."
            return
        }
        project = url
    }
    func clientExists(_ name: String) -> Bool {
        #if arch(arm64)
        let platform = "macos-arm64"
        #else
        let platform = "macos-x64"
        #endif
        return fileManager.isExecutableFile(atPath: root.appendingPathComponent("harnesses/\(name)/\(platform)/\(name)").path)
    }
    func launchClient(_ name: String) {
        guard ready, let project else { return }
        do {
            let connector = try executable("Connect \(name).command")
            let wrapper = try safeState("open-\(name.lowercased())-\(UUID().uuidString).command")
            let text = """
            #!/bin/zsh -f
            export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
            \(shellQuote(connector.path)) \(shellQuote(project.path))
            result=$?
            print '\(name) session ended. You can close this terminal.'
            print 'Press Return to close this launcher.'
            read -r answer
            /bin/rm -f -- "$0"
            exit $result
            """
            try Data(text.utf8).write(to: wrapper, options: .atomic)
            try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: wrapper.path)
            guard let terminal = NSWorkspace.shared.urlForApplication(withBundleIdentifier: "com.apple.Terminal") else {
                throw AppError(message: "Terminal is unavailable on this Mac.")
            }
            NSWorkspace.shared.open([wrapper], withApplicationAt: terminal, configuration: NSWorkspace.OpenConfiguration()) { _, failure in
                if let failure { Task { @MainActor in self.error = failure.localizedDescription } }
            }
        } catch { self.error = error.localizedDescription }
    }
    func copyReport() {
        let text = """
        Reflect Go · Phase \(phaseLabel)
        Hardware: \(inventory?.hardware.cpu ?? "Unknown")
        OS: \(inventory?.hardware.os ?? "Unknown")
        Memory: \(inventory.map { giB($0.hardware.physicalMemoryBytes) } ?? "Unknown")
        Engine: \(status)
        Model: \(preferences.model)
        Context: \(preferences.context)
        Compute: \(preferences.cpuOnly ? "CPU" : "Automatic / Metal when available")
        Endpoint: \(baseURL)
        Generation speed: \(speed.map { String(format: "%.2f tokens/s", $0) } ?? "Not measured")
        Engine resident memory: \(residentBytes.map(giB) ?? "Not measured")
        Startup: \(startSeconds.map { String(format: "%.1f seconds", $0) } ?? "Not measured")
        Error: \(error)
        """
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
    }
}

struct Card<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 14) { content }
            .padding(20).frame(maxWidth: .infinity, alignment: .leading)
            .fixedSize(horizontal: false, vertical: true)
            .background(surface).clipShape(RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(Color.white.opacity(0.065), lineWidth: 1))
    }
}
struct WorkstationView: View {
    @ObservedObject var model: Workstation
    @State private var advanced = false
    var body: some View {
        HStack(spacing: 0) {
            sidebar
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    header
                    if !model.notice.isEmpty {
                        HStack(alignment: .top) {
                            Label(model.notice, systemImage: "info.circle").font(.callout)
                            Spacer()
                            Button("Dismiss") { model.notice = "" }
                        }.padding(12).background(accent.opacity(0.08)).cornerRadius(10)
                    }
                    if !model.error.isEmpty {
                        Label(model.error, systemImage: "exclamationmark.triangle")
                            .foregroundColor(.orange).textSelection(.enabled).padding(14)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.orange.opacity(0.09)).cornerRadius(10)
                    }
                    if model.page == "Workstation" { dashboard }
                    else if model.page == "Diagnostics" { diagnostics }
                    else { guide }
                }.padding(28).frame(maxWidth: 1160)
            }.frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .background(canvasBackground).foregroundColor(Color(white: 0.93))
        .tint(accent).preferredColorScheme(.dark)
        .frame(minWidth: 980, minHeight: 720)
    }
    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                Image(systemName: "externaldrive.fill").font(.system(size: 28)).foregroundColor(accent)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Reflect Go").font(.system(size: 19, weight: .semibold))
                    Text("REFLECT ON THE GO").font(.system(size: 8, weight: .medium)).tracking(1.4).foregroundColor(.secondary)
                }
            }.padding(.bottom, 32)
            ForEach([("Workstation", "square.grid.2x2"), ("Diagnostics", "waveform.path.ecg"), ("Quick start", "book.closed")], id: \.0) { item in
                Button { model.page = item.0 } label: {
                    Label(item.0, systemImage: item.1).font(.system(size: 13, weight: .medium))
                        .frame(maxWidth: .infinity, alignment: .leading).padding(12)
                        .background(model.page == item.0 ? accent.opacity(0.12) : .clear).cornerRadius(9)
                }.buttonStyle(.plain).foregroundColor(model.page == item.0 ? accent : Color.secondary)
            }
            Spacer()
            Divider()
            Label("Carried on your drive", systemImage: "externaldrive").font(.caption).foregroundColor(.secondary)
            Text("Powered by this Mac").font(.caption).foregroundColor(.secondary)
            Text("PHASE \(phaseLabel)  /  WORKSTATION").font(.system(size: 9, weight: .medium)).tracking(1.2).foregroundColor(accent).padding(.top, 16)
        }.padding(22).frame(width: 208).background(Color.white.opacity(0.02))
    }
    private var header: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 7) {
                Text(model.page == "Workstation" ? "Your AI. Anywhere." : model.page)
                    .font(.system(size: 29, weight: .semibold))
                Text(model.page == "Workstation" ? "Your portable inference engine, ready for the way you work." : "Everything you need to keep your local workspace running.")
                    .font(.system(size: 12)).foregroundColor(.secondary)
            }
            Spacer()
            Label("ON THIS MAC", systemImage: "lock.shield").font(.system(size: 9, weight: .semibold)).tracking(1)
                .foregroundColor(accent).padding(10).background(accent.opacity(0.08)).cornerRadius(20)
        }
    }
    private func metric(_ title: String, _ value: String, _ footnote: String, icon: String) -> some View {
        Card {
            Label(title, systemImage: icon).font(.caption).foregroundColor(.secondary)
            Text(value).font(.system(size: 22, weight: .medium, design: .rounded)).lineLimit(1).minimumScaleFactor(0.65)
            Text(footnote).font(.system(size: 10)).foregroundColor(.secondary)
        }
    }
    private var dashboard: some View {
        VStack(spacing: 18) {
            HStack(spacing: 14) {
                metric("Available compute", model.inventory?.hardware.cpu ?? "Detecting…", model.inventory?.hardware.metalDevice == nil ? "CPU inference" : "Metal acceleration available", icon: "cpu")
                metric("Unified memory", model.inventory.map { giB($0.hardware.physicalMemoryBytes) } ?? "—", model.systemUsedBytes.map { giB($0) + " active + wired + compressed" } ?? "Reading this Mac’s memory", icon: "memorychip")
                metric("Model library", "\(model.inventory?.models.count ?? 0) models", "Shared by all phases · one copy each", icon: "externaldrive")
            }
            HStack(alignment: .top, spacing: 18) {
                VStack(spacing: 18) { library; settings }.frame(maxWidth: .infinity)
                VStack(spacing: 18) { engine; clients }.frame(maxWidth: .infinity)
            }
        }
    }
    private var library: some View {
        Card {
            HStack {
                Text("Choose your model").font(.system(size: 16, weight: .semibold))
                Spacer()
                Button { model.refresh() } label: { Image(systemName: "arrow.clockwise") }.disabled(model.active || model.loadingInventory).help("Refresh hardware and models")
            }
            if model.loadingInventory { ProgressView().controlSize(.small) }
            if model.inventory?.models.isEmpty == true { Text("Add a compatible text GGUF to the shared Models folder, then refresh.").font(.caption).foregroundColor(.secondary) }
            ForEach(model.inventory?.models ?? []) { item in
                Button {
                    model.preferences.model = item.name; model.persist()
                } label: {
                    HStack(alignment: .top, spacing: 10) {
                        Image(systemName: model.preferences.model == item.name ? "largecircle.fill.circle" : "circle").foregroundColor(accent)
                        VStack(alignment: .leading, spacing: 7) {
                            Text(item.title).font(.system(size: 12, weight: .medium)).multilineTextAlignment(.leading).fixedSize(horizontal: false, vertical: true)
                            Text("\(giB(item.bytes)) on disk · ~\(giB(item.estimatedBytes)) estimated").font(.system(size: 10)).foregroundColor(.secondary)
                            Text(item.issue ?? (item.eligible ? "Within this Mac’s memory budget" : "Try a smaller context or model"))
                                .font(.system(size: 10)).foregroundColor(item.eligible ? accent : .orange)
                        }
                        Spacer(minLength: 0)
                    }.padding(12).frame(maxWidth: .infinity, alignment: .leading)
                        .background(model.preferences.model == item.name ? accent.opacity(0.07) : Color.white.opacity(0.025))
                        .cornerRadius(10)
                        .overlay(RoundedRectangle(cornerRadius: 10).stroke(model.preferences.model == item.name ? accent.opacity(0.45) : .clear))
                }.buttonStyle(.plain).disabled(model.active || !item.eligible)
            }
            Text("Estimates reserve half of RAM for the Mac. They aren’t a measurement of free memory or a guarantee of fit.")
                .font(.system(size: 10)).foregroundColor(.secondary)
            Button("Open Models folder") { if let path = model.inventory?.modelsPath { NSWorkspace.shared.open(URL(fileURLWithPath: path)) } }.font(.caption)
        }
    }
    private var settings: some View {
        Card {
            Text("Run settings").font(.system(size: 16, weight: .semibold))
            Picker("Context", selection: $model.preferences.context) {
                ForEach([2048,4096,8192,16384,32768], id: \.self) { Text("\($0.formatted()) tokens").tag($0) }
            }.onChange(of: model.preferences.context) { _ in model.refresh() }
            Text("16,384 is the starting point for coding clients. A smaller context uses less memory but holds less of their conversation.")
                .font(.system(size: 10)).foregroundColor(.secondary)
            Picker("Compute", selection: $model.preferences.cpuOnly) {
                Text("Automatic · Metal when available").tag(false)
                Text("CPU only").tag(true)
            }
            DisclosureGroup("Advanced", isExpanded: $advanced) {
                VStack(alignment: .leading, spacing: 12) {
                    HStack { Text("Temperature"); Slider(value: $model.preferences.temperature, in: 0...2, step: 0.1); Text(String(format: "%.1f", model.preferences.temperature)).monospacedDigit() }
                    Picker("CPU threads", selection: $model.preferences.threads) {
                        Text("Automatic").tag(0)
                        ForEach(1...max(1, model.inventory?.hardware.cpuThreads ?? 1), id: \.self) { Text(String($0)).tag($0) }
                    }
                    HStack { Text("Local port"); Spacer(); TextField("8766", value: $model.preferences.port, formatter: Self.portFormatter).frame(width: 88) }
                    Text("Change the port if another engine is already using it.").font(.system(size: 10)).foregroundColor(.secondary)
                }.font(.caption).padding(.top, 10)
            }.font(.caption)
            if model.active { Text("Stop the engine to change models or settings.").font(.system(size: 10)).foregroundColor(accent) }
        }.disabled(model.active)
    }
    static var portFormatter: NumberFormatter {
        let formatter = NumberFormatter(); formatter.numberStyle = .none; formatter.allowsFloats = false
        return formatter
    }
    private var engine: some View {
        Card {
            HStack {
                Text("Portable inference engine").font(.system(size: 16, weight: .semibold))
                Spacer()
                Circle().fill(model.ready ? accent : model.active ? .orange : Color.secondary).frame(width: 8, height: 8)
            }
            Text(model.status).font(.system(size: 23, weight: .medium))
            Text(model.detail).font(.system(size: 11)).foregroundColor(.secondary).fixedSize(horizontal: false, vertical: true)
            if !model.active, let selected = model.selected, !selected.eligible {
                Text("This model does not pass the current memory or format check. Lower the context or choose an eligible model; see the model list for details.").font(.caption).foregroundColor(.orange)
            }
            if model.active && !model.ready { ProgressView().controlSize(.small) }
            HStack {
                if model.active {
                    Button("Stop & unload", systemImage: "stop.fill") { model.confirmStop() }.disabled(model.stopping)
                } else {
                    Button { model.start() } label: { Label("Start engine", systemImage: "play.fill").frame(maxWidth: .infinity).padding(.vertical, 4) }
                        .buttonStyle(.borderedProminent).tint(accent).foregroundColor(canvasBackground).disabled(!model.canStart)
                }
            }.controlSize(.large)
            Divider()
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 5) {
                    Text(model.speed.map { String(format: "%.1f", $0) } ?? "—").font(.system(size: 25, weight: .medium, design: .rounded)).monospacedDigit()
                    Text("tokens / second").font(.system(size: 10)).foregroundColor(.secondary)
                }
                Spacer()
                VStack(alignment: .leading, spacing: 5) {
                    Text(model.residentBytes.map(giB) ?? "—").font(.system(size: 25, weight: .medium, design: .rounded)).monospacedDigit()
                    Text("engine resident memory").font(.system(size: 10)).foregroundColor(.secondary)
                }
            }
            Text("Speed is the engine’s measured average after generation. Resident memory is a process reading; it may not include all GPU allocations.").font(.system(size: 10)).foregroundColor(.secondary)
            HStack {
                Button(model.testing ? "Generating…" : "Try a quick response") { model.quickTest() }.disabled(!model.ready || model.testing || model.requests > 0)
                Spacer()
                if model.generated > 0 { Text("\(model.generated) tokens generated").font(.system(size: 9)).foregroundColor(.secondary) }
            }
            if !model.response.isEmpty {
                Text(model.response).font(.system(size: 12)).textSelection(.enabled).padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading).background(accent.opacity(0.06)).cornerRadius(9)
            }
        }
    }
    private var clients: some View {
        Card {
            Text("Choose your portable AI client").font(.system(size: 16, weight: .semibold))
            Text("The client handles your project and tools. Reflect Go provides the local model.")
                .font(.system(size: 11)).foregroundColor(.secondary)
            Button { model.chooseProject() } label: {
                Label(model.project?.lastPathComponent ?? "Choose a project folder…", systemImage: "folder")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            if let project = model.project { Text(project.path).font(.system(size: 9)).foregroundColor(.secondary).lineLimit(2).textSelection(.enabled) }
            HStack(spacing: 10) {
                Button("Open Codex") { model.launchClient("Codex") }.disabled(!model.ready || model.project == nil || !model.codexInstalled)
                Button("Open Claude") { model.launchClient("Claude") }.disabled(!model.ready || model.project == nil || !model.claudeInstalled)
                Button("Open OpenCode") { model.launchClient("OpenCode") }.disabled(!model.ready || model.project == nil || !model.opencodeInstalled)
            }
            Text("Codex \(model.codexInstalled ? "on drive" : "missing") · Claude \(model.claudeInstalled ? "on drive" : "missing") · OpenCode \(model.opencodeInstalled ? "on drive" : "missing")")
                .font(.system(size: 10)).foregroundColor(.secondary)
            Text("All three open in Terminal. Use one client at a time. If Claude asks to use the API key supplied by Reflect Go, choose Yes for this local connection.")
                .font(.system(size: 10)).foregroundColor(.secondary)
            if model.ready { Text(model.baseURL).font(.system(size: 10, design: .monospaced)).foregroundColor(accent).textSelection(.enabled) }
        }
    }
    private var diagnostics: some View {
        VStack(spacing: 18) {
            Card {
                Text("Session health").font(.headline)
                Text("\(model.status) · \(model.baseURL)").textSelection(.enabled)
                Text("Model: \(model.preferences.model)\nContext: \(model.preferences.context) tokens\nStartup: \(model.startSeconds.map { String(format: "%.1f seconds", $0) } ?? "Not measured")")
                    .font(.caption).foregroundColor(.secondary).textSelection(.enabled)
                HStack {
                    Button("Copy diagnostics") { model.copyReport() }
                    Button(model.verifying ? "Checking…" : "Check file integrity") { model.verify() }.disabled(model.verifying || model.active)
                    Button("Refresh") { model.readLog(); model.refresh() }
                }
                Text("Integrity checks run while the engine is stopped. The copied report excludes the local key and engine log.").font(.caption).foregroundColor(.secondary)
                if !model.verification.isEmpty { ScrollView { Text(model.verification).font(.system(size: 10, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading) }.frame(maxHeight: 150) }
            }
            Card {
                Text("Engine log").font(.headline)
                Text("Recent output from this app’s engine. Logs can contain client prompts; review before sharing.").font(.caption).foregroundColor(.secondary)
                ScrollView([.vertical, .horizontal]) { Text(model.log).font(.system(size: 10, design: .monospaced)).textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading) }.frame(height: 340)
            }
        }
    }
    private var guide: some View {
        VStack(spacing: 18) {
            Card {
                Text("From drive to workstation").font(.system(size: 21, weight: .medium))
                instruction("1", "Start your engine", "Choose a model, keep the automatic compute setting, and click Start engine. Wait for Engine ready. The app keeps the engine running for you.")
                instruction("2", "Open your client", "Choose a project outside the Reflect Go collection, then open Codex, Claude or OpenCode. Each portable client opens its interactive chat in Terminal.")
                instruction("3", "Approve the local connection", "Claude may ask whether to use the supplied API key, with No recommended. When launched through Reflect Go, choose Yes to use this local engine. This is Claude’s confirmation screen; the key belongs to the local provider.")
                instruction("4", "Finish and unplug", "Quit your client, click Stop & unload, then quit Reflect Go before ejecting. Closing this app’s window asks to stop its engine. If the app crashes or is force-quit, its supervisor unloads the engine within about ten seconds. Engines started from other windows or phase folders keep running.")
            }
            Card {
                Text("A few useful things to know").font(.headline)
                Text("• Models live in the shared Models folder beside the phases. Keep the collection together when moving it.\n\n• Your Mac supplies compute and memory. You don’t need to install a separate inference runtime. Codex, Claude Code and OpenCode run from the drive. Their configuration and sessions stay in this phase’s state folder. Your project’s build tools still come from the computer.\n\n• The 4B model is the coding starting point. The 1.5B model is a lighter chat demonstration. Local models can make mistakes; review file changes.\n\n• A busy port means another engine is using that address. Stop your other engine when you finish with it, or choose a different port under Advanced. Multiple loaded models share this Mac’s RAM.\n\n• If startup fails, check Diagnostics. Free disk space, available memory, macOS security approval, and keeping all phase files together are the first things to check.")
                    .font(.system(size: 12)).foregroundColor(.secondary).fixedSize(horizontal: false, vertical: true)
            }
        }
    }
    private func instruction(_ number: String, _ title: String, _ text: String) -> some View {
        HStack(alignment: .top, spacing: 16) {
            Text(number).font(.system(size: 17, weight: .medium, design: .rounded)).foregroundColor(accent)
                .frame(width: 32, height: 32).background(accent.opacity(0.1)).clipShape(Circle())
            VStack(alignment: .leading, spacing: 6) {
                Text(title).font(.system(size: 14, weight: .semibold))
                Text(text).font(.system(size: 12)).foregroundColor(.secondary).fixedSize(horizontal: false, vertical: true)
            }
        }.padding(.vertical, 8)
    }
}

@MainActor final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    var window: NSWindow!
    var workstation: Workstation!
    func applicationDidFinishLaunching(_ notification: Notification) {
        // App bundle sits directly inside its phase folder. All inference paths are relative.
        let root = Bundle.main.bundleURL.deletingLastPathComponent()
        workstation = Workstation(root: root)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1180, height: 880),
                          styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "Reflect Go · Portable AI Workstation"
        window.minSize = NSSize(width: 1000, height: 760)
        window.contentView = NSHostingView(rootView: WorkstationView(model: workstation))
        window.backgroundColor = NSColor(red: 0.065, green: 0.078, blue: 0.074, alpha: 1)
        window.appearance = NSAppearance(named: .darkAqua)
        window.delegate = self
        window.center()
        window.makeKeyAndOrderFront(nil)
        let menu = NSMenu(), appMenu = NSMenu()
        let parent = NSMenuItem(); parent.submenu = appMenu; menu.addItem(parent)
        appMenu.addItem(withTitle: "About Reflect Go", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Quit Reflect Go", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        let editItem = NSMenuItem(title: "Edit", action: nil, keyEquivalent: "")
        let edit = NSMenu(title: "Edit"); editItem.submenu = edit; menu.addItem(editItem)
        for (title, action, key) in [("Copy", "copy:", "c"), ("Paste", "paste:", "v"), ("Select All", "selectAll:", "a")] {
            edit.addItem(withTitle: title, action: Selector(action), keyEquivalent: key)
        }
        NSApp.mainMenu = menu
        NSApp.activate(ignoringOtherApps: true)
    }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply { workstation.requestQuit() }
    func windowShouldClose(_ sender: NSWindow) -> Bool { NSApp.terminate(nil); return false }
}

#if !WORKSTATION_TEST
@main struct ReflectGo {
    @MainActor static func main() {
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.setActivationPolicy(.regular)
        app.delegate = delegate
        withExtendedLifetime(delegate) { app.run() }
    }
}

#endif
