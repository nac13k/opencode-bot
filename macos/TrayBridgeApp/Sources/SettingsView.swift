import AppKit
import SwiftUI

struct SettingsView: View {
  @ObservedObject var serviceManager: ServiceManager

  var body: some View {
    VStack(alignment: .leading, spacing: 14) {
      Text("OpenCode Telegram Bridge")
        .font(.title2)
        .bold()
      Text("Menu bar controller to start, stop, and monitor the Telegram bridge service.")
        .font(.subheadline)
        .foregroundStyle(.secondary)

      GroupBox("Configuration") {
        VStack(alignment: .leading, spacing: 10) {
          HStack {
            Text("BOT_TOKEN")
              .frame(width: 110, alignment: .leading)
            SecureField("Telegram bot token", text: $serviceManager.botToken)
          }

          HStack {
            Text("ADMIN_USER_IDS")
              .frame(width: 110, alignment: .leading)
            TextField("123456,987654", text: $serviceManager.adminUserIds)
          }

          HStack {
            Text("Transport")
              .frame(width: 110, alignment: .leading)
            Picker("Transport", selection: $serviceManager.botTransport) {
              Text("polling").tag("polling")
              Text("webhook").tag("webhook")
            }
            .pickerStyle(.segmented)
          }

          HStack {
            Text("DATA_DIR")
              .frame(width: 110, alignment: .leading)
            TextField("./data", text: $serviceManager.dataDir)
          }

          HStack {
            Text("OPENCODE_COMMAND")
              .frame(width: 110, alignment: .leading)
            TextField("opencode", text: $serviceManager.opencodeCommand)
          }

          HStack {
            Text("TIMEOUT_MS")
              .frame(width: 110, alignment: .leading)
            TextField("120000", value: $serviceManager.opencodeTimeoutMs, format: .number)
          }

          DisclosureGroup("Advanced", isExpanded: $serviceManager.showAdvanced) {
            HStack {
              Text("Project path")
                .frame(width: 110, alignment: .leading)
              TextField("/path/to/project", text: $serviceManager.projectPath)
              Button("Browse") {
                selectProjectPath()
              }
            }
            Text("This path is only used to locate the repo where `npm run dev` is executed.")
              .font(.caption)
              .foregroundStyle(.secondary)
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
      serviceManager.updateProjectPath(url.path)
    }
  }
}
