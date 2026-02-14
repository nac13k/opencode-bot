import AppKit
import SwiftUI

struct SettingsView: View {
  @ObservedObject var serviceManager: ServiceManager

  var body: some View {
    VStack(alignment: .leading, spacing: 14) {
      Text("OpenCode Telegram Bridge")
        .font(.title2)
        .bold()

      GroupBox("Configuration") {
        VStack(alignment: .leading, spacing: 10) {
          HStack {
            Text("Project path")
              .frame(width: 110, alignment: .leading)
            TextField("/path/to/project", text: $serviceManager.projectPath)
            Button("Browse") {
              selectProjectPath()
            }
          }

          HStack {
            Text("Start command")
              .frame(width: 110, alignment: .leading)
            TextField("npm run dev", text: $serviceManager.startCommand)
          }

          VStack(alignment: .leading, spacing: 4) {
            Text("Extra env (KEY=VALUE per line)")
            TextEditor(text: $serviceManager.extraEnv)
              .font(.system(.body, design: .monospaced))
              .frame(height: 90)
              .overlay(
                RoundedRectangle(cornerRadius: 6)
                  .stroke(Color.gray.opacity(0.3), lineWidth: 1)
              )
          }

          HStack {
            Button("Save") {
              serviceManager.saveConfig()
            }

            if serviceManager.isRunning {
              Button("Stop") {
                serviceManager.stopService()
              }
            } else {
              Button("Start") {
                serviceManager.startService()
              }
            }

            Button("Restart") {
              serviceManager.restartService()
            }

            Spacer()
          }
        }
        .padding(.vertical, 4)
      }

      GroupBox("Service") {
        HStack {
          Label(serviceManager.isRunning ? "Running" : "Stopped", systemImage: serviceManager.isRunning ? "checkmark.circle.fill" : "xmark.circle")
            .foregroundStyle(serviceManager.isRunning ? .green : .red)
          Spacer()
          Text(serviceManager.statusText)
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(.vertical, 2)
      }

      GroupBox("Logs") {
        VStack(alignment: .leading, spacing: 8) {
          ScrollView {
            Text(serviceManager.logs.isEmpty ? "No logs yet" : serviceManager.logs)
              .font(.system(.caption, design: .monospaced))
              .frame(maxWidth: .infinity, alignment: .leading)
              .padding(8)
          }
          .frame(minHeight: 200)
          .background(Color.black.opacity(0.06))
          .clipShape(RoundedRectangle(cornerRadius: 8))

          HStack {
            Button("Clear Logs") {
              serviceManager.clearLogs()
            }
            Spacer()
          }
        }
      }
    }
    .padding(16)
  }

  private func selectProjectPath() {
    let panel = NSOpenPanel()
    panel.canChooseDirectories = true
    panel.canChooseFiles = false
    panel.canCreateDirectories = true
    panel.allowsMultipleSelection = false
    if panel.runModal() == .OK, let url = panel.url {
      serviceManager.projectPath = url.path
    }
  }
}
