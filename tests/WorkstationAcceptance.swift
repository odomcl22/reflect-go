import AppKit
import Foundation
import Darwin

struct CheckFailure: Error { let message: String }
@main struct WorkstationAcceptance {
    @MainActor static func require(_ condition: Bool, _ message: String) throws {
        if !condition { throw CheckFailure(message: message) }
        print("PASS: \(message)"); fflush(stdout)
    }
    @MainActor static func wait(_ model: Workstation, seconds: Double = 90, until: () -> Bool) async throws {
        let deadline = Date().addingTimeInterval(seconds)
        while !until() && Date() < deadline {
            await model.tick()
            try await Task.sleep(nanoseconds: 150_000_000)
        }
        if !until() { throw CheckFailure(message: "Timed out: \(model.status) / \(model.error)") }
    }
    static func reserve(_ port: Int) -> Int32 {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        var reuse: Int32 = 1
        setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse, socklen_t(MemoryLayout<Int32>.size))
        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = in_port_t(port).bigEndian
        address.sin_addr.s_addr = inet_addr("127.0.0.1")
        let code = withUnsafePointer(to: &address) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { bind(fd, $0, socklen_t(MemoryLayout<sockaddr_in>.size)) }
        }
        if code != 0 { close(fd); return -1 }
        listen(fd, 1)
        return fd
    }
    @MainActor static func main() async {
        let root = URL(fileURLWithPath: CommandLine.arguments[1])
        let model = Workstation(root: root)
        do {
            try await wait(model, until: { !model.loadingInventory })
            try require(model.inventory?.models.count ?? 0 >= 2, "real shared models discovered")
            try require(model.notice.contains("Defaults"), "corrupt preferences explain restored defaults")
            let large = "Qwen3-4B-Instruct-2507-Q4_K_M.gguf"
            let small = "qwen2.5-1.5b-instruct-q4_k_m.gguf"
            model.preferences.model = large
            model.preferences.context = 32768; model.refresh()
            try await wait(model, until: { !model.loadingInventory })
            try require(model.preferences.model == large && !model.canStart, "context does not silently switch an ineligible model")
            model.preferences.context = 2048; model.preferences.model = small; model.refresh()
            try await wait(model, until: { !model.loadingInventory })
            model.preferences.port = 18876
            let occupied = reserve(model.preferences.port)
            try require(occupied >= 0, "test owns its occupied-port listener")
            model.start()
            try await wait(model, until: { !model.active })
            try require(model.error.contains("already in use"), "occupied port gives actionable error")
            try require(fcntl(occupied, F_GETFD) >= 0, "foreign listener survives failure")
            close(occupied)
            model.start()
            try await wait(model, until: { model.ready || !model.active })
            try require(model.ready, "small model starts and authenticated metrics become ready")
            model.quickTest()
            try await wait(model, until: { !model.testing })
            await model.tick(); model.sampleMemory()
            try require(!model.response.isEmpty && model.generated > 0, "real local generation completes")
            try require(model.speed ?? 0 > 0 && model.residentBytes ?? 0 > 100_000_000, "speed and inference-child memory remain measured")
            print("MEASURED: \(model.speed ?? 0) tokens/s; \(model.residentBytes ?? 0) resident bytes")
            let second = Workstation(root: root)
            try await wait(second, until: { !second.loadingInventory })
            second.start()
            try require(!second.active && second.error.contains("another app window"), "second app cannot replace the running session")
            model.stop()
            try await wait(model, seconds: 12, until: { !model.active })
            let released = reserve(model.preferences.port)
            try require(released >= 0, "normal stop releases listener")
            close(released)
            model.preferences.model = large; model.preferences.context = 16384; model.refresh()
            try await wait(model, until: { !model.loadingInventory })
            model.start()
            try await wait(model, until: { model.ready || !model.active })
            try require(model.ready, "switch to 4B model and restart succeeds")
            try require(FileManager.default.fileExists(atPath: root.appendingPathComponent("state/workstation-previous.log").path), "previous session log retained")
            if CommandLine.arguments.contains("--crash") {
                print("CRASHING TEST OWNER"); fflush(stdout)
                kill(getpid(), SIGKILL)
            }
            model.stop()
            try await wait(model, seconds: 12, until: { !model.active })
            print("PASS: all workstation acceptance checks"); fflush(stdout)
            exit(0)
        } catch {
            print("FAIL: \(error)"); fflush(stdout)
            model.stop()
            try? await wait(model, seconds: 12, until: { !model.active })
            exit(1)
        }
    }
}
