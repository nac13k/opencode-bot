import SwiftUI

struct SettingsView: View {
  @ObservedObject var serviceManager: ServiceManager
  @State private var newAdminId = ""
  @State private var newAllowedId = ""

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

          VStack(alignment: .leading, spacing: 8) {
            Text("ADMIN_USER_IDS")
              .font(.caption)
              .foregroundStyle(.secondary)
            chipGrid(ids: parsedIds(from: serviceManager.adminUserIds)) { id in
              serviceManager.adminUserIds = removeId(id, from: serviceManager.adminUserIds)
            }
            HStack {
              TextField("Agregar admin id", text: $newAdminId)
              Button("Add") {
                serviceManager.adminUserIds = addId(newAdminId, to: serviceManager.adminUserIds)
                newAdminId = ""
              }
            }
          }

          VStack(alignment: .leading, spacing: 8) {
            Text("ALLOWED_USER_IDS")
              .font(.caption)
              .foregroundStyle(.secondary)
            chipGrid(ids: parsedIds(from: serviceManager.allowedUserIds)) { id in
              serviceManager.allowedUserIds = removeId(id, from: serviceManager.allowedUserIds)
            }
            HStack {
              TextField("Agregar allowed id", text: $newAllowedId)
              Button("Add") {
                serviceManager.allowedUserIds = addId(newAllowedId, to: serviceManager.allowedUserIds)
                newAllowedId = ""
              }
            }
          }

          HStack {
            Text("Transport")
              .frame(width: 110, alignment: .leading)
            Picker("Transport", selection: $serviceManager.botTransport) {
              Text("polling").tag(BotTransport.polling)
              Text("webhook").tag(BotTransport.webhook)
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

          Divider()

          Text("OpenCode Server")
            .font(.subheadline)
            .bold()

          HStack {
            Text("OPENCODE_SERVER_URL")
              .frame(width: 110, alignment: .leading)
            TextField("http://127.0.0.1:4096", text: $serviceManager.opencodeServerUrl)
          }

          HStack {
            Text("OPENCODE_SERVER_USERNAME")
              .frame(width: 110, alignment: .leading)
            TextField("opencode", text: $serviceManager.opencodeServerUsername)
          }

          HStack {
            Text("OPENCODE_SERVER_PASSWORD")
              .frame(width: 110, alignment: .leading)
            SecureField("(optional)", text: $serviceManager.opencodeServerPassword)
          }

          HStack {
            Text("NODE_BINARY")
              .frame(width: 110, alignment: .leading)
            TextField("/opt/homebrew/bin/node", text: $serviceManager.nodeBinaryPath)
          }

          HStack {
            Text("TIMEOUT_MS")
              .frame(width: 110, alignment: .leading)
            TextField("120000", value: $serviceManager.opencodeTimeoutMs, format: .number)
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
          Text(serviceManager.usingBundledServer ? "Bundled" : "Unavailable")
            .font(.caption)
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .background(serviceManager.usingBundledServer ? Color.blue.opacity(0.18) : Color.red.opacity(0.18))
            .clipShape(Capsule())
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
              .textSelection(.enabled)
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
            Button("Copy Logs") {
              let pasteboard = NSPasteboard.general
              pasteboard.clearContents()
              pasteboard.setString(serviceManager.logs, forType: .string)
            }
            Spacer()
          }
        }
      }
    }
    .padding(16)
    .alert("Node requerido", isPresented: Binding(
      get: { serviceManager.alertMessage != nil },
      set: { value in if !value { serviceManager.alertMessage = nil } }
    )) {
      Button("OK", role: .cancel) {}
    } message: {
      Text(serviceManager.alertMessage ?? "")
    }
  }

  private func parsedIds(from raw: String) -> [String] {
    let list = raw
      .split(separator: ",")
      .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
      .filter { !$0.isEmpty }
    return Array(Set(list)).sorted()
  }

  private func addId(_ value: String, to raw: String) -> String {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else { return raw }
    var current = parsedIds(from: raw)
    current.append(trimmed)
    return Array(Set(current)).sorted().joined(separator: ",")
  }

  private func removeId(_ value: String, from raw: String) -> String {
    let next = parsedIds(from: raw).filter { $0 != value }
    return next.joined(separator: ",")
  }

  private func chipGrid(ids: [String], onRemove: @escaping (String) -> Void) -> some View {
    LazyVGrid(columns: [GridItem(.adaptive(minimum: 120), alignment: .leading)], alignment: .leading) {
      ForEach(ids, id: \.self) { id in
        HStack(spacing: 6) {
          Text(id)
            .font(.caption)
            .lineLimit(1)
          Button {
            onRemove(id)
          } label: {
            Image(systemName: "xmark.circle.fill")
              .font(.caption)
              .foregroundStyle(.secondary)
          }
          .buttonStyle(.plain)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 4)
        .background(Color.gray.opacity(0.15))
        .clipShape(Capsule())
      }
    }
  }
}
