import AppKit
import SwiftUI

@main
struct TrayBridgeApp: App {
  @StateObject private var serviceManager = ServiceManager()

  init() {
    NSApplication.shared.setActivationPolicy(.accessory)
  }

  var body: some Scene {
    MenuBarExtra("OpenCode Bridge", systemImage: serviceManager.isRunning ? "bolt.circle.fill" : "bolt.circle") {
      TrayMenuView(serviceManager: serviceManager)
    }

    Window("Bridge Settings", id: "settings") {
      SettingsView(serviceManager: serviceManager)
        .frame(minWidth: 680, minHeight: 500)
    }
  }
}

private struct TrayMenuView: View {
  @ObservedObject var serviceManager: ServiceManager
  @Environment(\.openWindow) private var openWindow

  var body: some View {
    VStack(alignment: .leading, spacing: 10) {
      Text(serviceManager.isRunning ? "Status: Running" : "Status: Stopped")
      Text(serviceManager.statusText)
        .font(.caption)
        .foregroundStyle(.secondary)
        .lineLimit(2)

      Divider()

      Button("Open Settings") {
        openWindow(id: "settings")
      }

      if serviceManager.isRunning {
        Button("Stop Service") {
          serviceManager.stopService()
        }
      } else {
        Button("Start Service") {
          serviceManager.startService()
        }
      }

      Button("Restart Service") {
        serviceManager.restartService()
      }

      Divider()

      Button("Quit") {
        NSApplication.shared.terminate(nil)
      }
    }
    .padding(8)
  }
}
