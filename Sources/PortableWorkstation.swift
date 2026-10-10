import Cocoa
import WebKit

// Native Mac window for the same authenticated workstation used on Windows and
// Linux. Keep one implementation of model, client, download, and chat behavior.
@MainActor final class PortableDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKUIDelegate, NSWindowDelegate {
    private var window: NSWindow!
    private var web: WKWebView!
    private var process: Process?
    private var owner: Pipe?
    private var timer: Timer?
    private var base: URL?
    private var quitting = false
    private var started = Date()

    func applicationDidFinishLaunching(_ notification: Notification) {
        let root = Bundle.main.bundleURL.deletingLastPathComponent()
        #if arch(arm64)
        let platform = "macos-arm64"
        #else
        let platform = "macos-x64"
        #endif
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        web.uiDelegate = self
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1180, height: 880), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "Reflect Go"
        window.minSize = NSSize(width: 720, height: 560)
        window.contentView = web
        window.delegate = self
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        let menu = NSMenu(), appMenu = NSMenu(), parent = NSMenuItem()
        parent.submenu = appMenu; menu.addItem(parent)
        appMenu.addItem(withTitle: "About Reflect Go", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Quit Reflect Go", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        let editMenu = NSMenu(title: "Edit"), editItem = NSMenuItem(title: "Edit", action: nil, keyEquivalent: "")
        editItem.submenu = editMenu; menu.addItem(editItem)
        for (title, selector, key) in [("Copy", "copy:", "c"), ("Paste", "paste:", "v"), ("Select All", "selectAll:", "a")] {
            editMenu.addItem(withTitle: title, action: NSSelectorFromString(selector), keyEquivalent: key)
        }
        NSApp.mainMenu = menu
        let child = Process(), pipe = Pipe()
        child.executableURL = root.appendingPathComponent("bin/\(platform)/reflectgo")
        child.arguments = ["ui"]
        var env = ProcessInfo.processInfo.environment
        env["REFLECT_GO_NO_BROWSER"] = "1"
        env["REFLECT_GO_UI_OWNED"] = "1"
        child.environment = env
        child.standardInput = pipe
        child.standardOutput = FileHandle.nullDevice
        child.standardError = FileHandle.nullDevice
        process = child; owner = pipe
        do { try child.run() } catch { fail("Could not open the portable workstation. Keep the app beside its bin and runtime folders.\n\(error.localizedDescription)"); return }
        started = Date()
        timer = Timer.scheduledTimer(withTimeInterval: 0.25, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.check(root) }
        }
    }
    private func check(_ root: URL) {
        guard let process else { return }
        if let data = try? Data(contentsOf: root.appendingPathComponent("state/appearance.json")), let value = (try? JSONSerialization.jsonObject(with: data)) as? [String: String] {
            let theme = value["theme"]
            window.appearance = theme == "dark" ? NSAppearance(named: .darkAqua) : theme == "light" ? NSAppearance(named: .aqua) : nil
        }
        if !process.isRunning {
            timer?.invalidate()
            if quitting { NSApp.reply(toApplicationShouldTerminate: true) }
            else if base != nil { NSApp.terminate(nil) }
            else { fail("The workstation could not start. Another instance may be open. Close it and reopen Reflect Go, or use Reflect Go.sh to see the startup error.") }
            return
        }
        if base == nil {
            let session = root.appendingPathComponent("state/browser-session.json")
            if let data = try? Data(contentsOf: session),
               let value = (try? JSONSerialization.jsonObject(with: data)) as? [String: String],
               let address = value["base_url"], let token = value["token"],
               let url = URL(string: address), url.host == "127.0.0.1", url.scheme == "http",
               token.count == 64, token.allSatisfy({ $0.isHexDigit }),
               let authenticated = URL(string: address + "/#" + token) {
                base = url
                web.load(URLRequest(url: authenticated))
            } else if Date().timeIntervalSince(started) > 20 {
                owner?.fileHandleForWriting.closeFile()
                fail("Startup timed out. Check that the drive is writable and the complete package is present.")
            }
        }
    }
    private func fail(_ message: String) {
        timer?.invalidate()
        let alert = NSAlert(); alert.messageText = "Reflect Go could not start"; alert.informativeText = message; alert.runModal()
        owner?.fileHandleForWriting.closeFile()
        NSApp.terminate(nil)
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
        if url.host == base?.host && url.port == base?.port && url.scheme == "http" { decisionHandler(.allow) }
        else { decisionHandler(.cancel); if navigationAction.navigationType == .linkActivated && url.scheme == "https" { NSWorkspace.shared.open(url) } }
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert(); alert.messageText = "Reflect Go"; alert.informativeText = message
        alert.addButton(withTitle: "Continue"); alert.addButton(withTitle: "Cancel")
        alert.beginSheetModal(for: window) { result in completionHandler(result == .alertFirstButtonReturn) }
    }
    func webView(_ webView: WKWebView, runOpenPanelWith parameters: WKOpenPanelParameters, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping ([URL]?) -> Void) {
        let panel = NSOpenPanel(); panel.canChooseDirectories = false; panel.allowsMultipleSelection = false
        panel.beginSheetModal(for: window) { result in completionHandler(result == .OK ? panel.urls : nil) }
    }
    func windowShouldClose(_ sender: NSWindow) -> Bool { NSApp.terminate(nil); return false }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let process, process.isRunning else { return .terminateNow }
        if quitting { return .terminateLater }
        let alert = NSAlert(); alert.messageText = "Stop Reflect Go and close?"; alert.informativeText = "Finish client tasks first. The engine will stop and active downloads will pause before the drive can be ejected."
        alert.addButton(withTitle: "Stop and quit"); alert.addButton(withTitle: "Keep running")
        if alert.runModal() != .alertFirstButtonReturn { return .terminateCancel }
        quitting = true
        timer?.invalidate()
        // Cocoa's deferred-termination loop does not reliably run ordinary
        // timers. Reply from the child's exit notification instead.
        process.terminationHandler = { _ in
            DispatchQueue.main.async { NSApp.reply(toApplicationShouldTerminate: true) }
        }
        owner?.fileHandleForWriting.closeFile()
        return .terminateLater
    }
}
@main struct PortableReflectGo {
    static func main() { let app = NSApplication.shared; let delegate = PortableDelegate(); app.setActivationPolicy(.regular); app.delegate = delegate; withExtendedLifetime(delegate) { app.run() } }
}
