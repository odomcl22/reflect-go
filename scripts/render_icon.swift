import Cocoa

// The same drive mark used by the workstation, drawn as a scalable app icon.
let directory = URL(fileURLWithPath: CommandLine.arguments[1])
try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
for size in [16, 32, 48, 64, 128, 256, 512, 1024] {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size, bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    let context = NSGraphicsContext.current!.cgContext
    context.scaleBy(x: CGFloat(size)/1024, y: CGFloat(size)/1024)
    let background = NSBezierPath(roundedRect: NSRect(x: 20, y: 20, width: 984, height: 984), xRadius: 218, yRadius: 218)
    NSGradient(starting: NSColor(srgbRed: 0.06, green: 0.12, blue: 0.09, alpha: 1), ending: NSColor(srgbRed: 0.20, green: 0.36, blue: 0.26, alpha: 1))!.draw(in: background, angle: 55)
    NSColor(srgbRed: 0.73, green: 0.88, blue: 0.76, alpha: 1).setStroke()
    let drive = NSBezierPath(roundedRect: NSRect(x: 165, y: 280, width: 694, height: 460), xRadius: 100, yRadius: 100)
    drive.lineWidth = 45; drive.stroke()
    let slot = NSBezierPath(); slot.move(to: NSPoint(x: 260, y: 570)); slot.line(to: NSPoint(x: 765, y: 570)); slot.lineWidth = 35; slot.lineCapStyle = .round; slot.stroke()
    NSColor(srgbRed: 0.73, green: 0.88, blue: 0.76, alpha: 1).setFill()
    NSBezierPath(ovalIn: NSRect(x: 692, y: 365, width: 54, height: 54)).fill()
    NSBezierPath(ovalIn: NSRect(x: 597, y: 365, width: 54, height: 54)).fill()
    NSGraphicsContext.restoreGraphicsState()
    try rep.representation(using: .png, properties: [:])!.write(to: directory.appendingPathComponent("icon-\(size).png"))
}
