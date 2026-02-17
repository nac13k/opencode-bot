import SwiftUI

struct SettingsView: View {
  @ObservedObject var serviceManager: ServiceManager
  @State private var newAdminId = ""
  @State private var newAllowedId = ""

  var body: some View {
    ScrollView {
      VStack(alignment: .leading, spacing: 14) {
        Text("opencode-bot")
          .font(.title2)
          .bold()
        Text("Panel completo para controlar el bridge Go y ejecutar comandos desde la app.")
          .font(.subheadline)
          .foregroundStyle(.secondary)

        GroupBox("Configuration") {
          VStack(alignment: .leading, spacing: 10) {
            labeledSecureField("Bot token", placeholder: "123456789:ABC...", text: $serviceManager.botToken)

            VStack(alignment: .leading, spacing: 8) {
              Text("Admin user IDs")
                .font(.caption)
                .foregroundStyle(.secondary)
              chipGrid(ids: parsedIds(from: serviceManager.adminUserIds)) { id in
                serviceManager.adminUserIds = removeId(id, from: serviceManager.adminUserIds)
              }
              HStack {
                TextField("Add admin ID", text: $newAdminId)
                Button("Add") {
                  serviceManager.adminUserIds = addId(newAdminId, to: serviceManager.adminUserIds)
                  newAdminId = ""
                }
              }
            }

            VStack(alignment: .leading, spacing: 8) {
              Text("Allowed user IDs")
                .font(.caption)
                .foregroundStyle(.secondary)
              chipGrid(ids: parsedIds(from: serviceManager.allowedUserIds)) { id in
                serviceManager.allowedUserIds = removeId(id, from: serviceManager.allowedUserIds)
              }
              HStack {
                TextField("Add allowed ID", text: $newAllowedId)
                Button("Add") {
                  serviceManager.allowedUserIds = addId(newAllowedId, to: serviceManager.allowedUserIds)
                  newAllowedId = ""
                }
              }
            }

            HStack {
              Text("Transport")
                .frame(width: 170, alignment: .leading)
              Picker("Transport", selection: $serviceManager.botTransport) {
                Text("polling").tag(BotTransport.polling)
                Text("webhook").tag(BotTransport.webhook)
              }
              .pickerStyle(.segmented)
            }

            labeledField("Webhook URL", placeholder: "https://your.domain/telegram/webhook", text: $serviceManager.webhookUrl)
            labeledField("Webhook listen", placeholder: ":8090", text: $serviceManager.webhookListenAddr)
            labeledField("Data directory", placeholder: "./data", text: $serviceManager.dataDir)

            Divider()

            Text("OpenCode")
              .font(.subheadline)
              .bold()
            labeledField("Server URL", placeholder: "http://127.0.0.1:4096", text: $serviceManager.opencodeServerUrl)
            labeledField("Username", placeholder: "opencode", text: $serviceManager.opencodeServerUsername)
            labeledSecureField("Password", placeholder: "Optional", text: $serviceManager.opencodeServerPassword)
            labeledField("Default session", placeholder: "ses_...", text: $serviceManager.defaultSessionId)
            labeledNumberField("Timeout (ms)", value: $serviceManager.opencodeTimeoutMs)
            labeledNumberField("Health port", value: $serviceManager.healthPort)

            Divider()

            Text("Relay")
              .font(.subheadline)
              .bold()
            HStack {
              Text("Mode")
                .frame(width: 170, alignment: .leading)
              Picker("Relay mode", selection: $serviceManager.relayMode) {
                Text("last").tag(RelayMode.last)
                Text("final").tag(RelayMode.final)
              }
              .pickerStyle(.segmented)
            }
            Toggle("Fallback (solo para final)", isOn: $serviceManager.relayFallback)
            labeledNumberField("Fallback delay (ms)", value: $serviceManager.relayFallbackDelayMs)

            HStack {
              Button("Save") { serviceManager.saveConfig() }
              if serviceManager.isRunning {
                Button("Stop") { serviceManager.stopService() }
              } else {
                Button("Start") { serviceManager.startService() }
              }
              Button("Restart") { serviceManager.restartService() }
              Spacer()
            }
          }
          .padding(.vertical, 4)
        }

        GroupBox("Service") {
          HStack {
            Label(serviceManager.isRunning ? "Running" : "Stopped", systemImage: serviceManager.isRunning ? "checkmark.circle.fill" : "xmark.circle")
              .foregroundStyle(serviceManager.isRunning ? .green : .red)
            Text(serviceManager.usingBundledServer ? "Bundled bridge" : "Unavailable")
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

        GroupBox("Comandos y Interaccion") {
          VStack(alignment: .leading, spacing: 10) {
            Text("Estos botones ejecutan los endpoints locales del bridge y muestran respuesta completa JSON.")
              .font(.caption)
              .foregroundStyle(.secondary)

            HStack {
              Text("Chat ID")
                .frame(width: 170, alignment: .leading)
              TextField("123456", text: $serviceManager.commandChatId)
            }
            HStack {
              Text("User ID")
                .frame(width: 170, alignment: .leading)
              TextField("123456", text: $serviceManager.commandUserId)
            }
            HStack {
              Text("Target user ID")
                .frame(width: 170, alignment: .leading)
              TextField("654321", text: $serviceManager.commandTargetUserId)
            }
            HStack {
              Text("Session ID")
                .frame(width: 170, alignment: .leading)
              TextField("ses_...", text: $serviceManager.commandSessionId)
            }
            HStack {
              Text("Model ID")
                .frame(width: 170, alignment: .leading)
              TextField("provider/model", text: $serviceManager.commandModelId)
            }
            HStack {
              Text("Resolve usernames")
                .frame(width: 170, alignment: .leading)
              TextField("@a, @b", text: $serviceManager.commandUsernames)
            }

            Divider()

            commandButtonsRow(title: "Estado") {
              Button("Status") { serviceManager.runStatusCommand() }
              Button("Access List") { serviceManager.runListAccessCommand() }
            }
            commandButtonsRow(title: "Sesion") {
              Button("Session Get") { serviceManager.runSessionCurrentCommand() }
              Button("Session List") { serviceManager.runSessionListCommand() }
              Button("Session Use") { serviceManager.runSessionUseCommand() }
              Button("Session New") { serviceManager.runSessionNewCommand() }
              Button("Compact") { serviceManager.runCompactCommand() }
            }
            commandButtonsRow(title: "Modelos") {
              Button("Models List") { serviceManager.runModelsListCommand() }
              Button("Models Set") { serviceManager.runModelsSetCommand() }
              Button("Models Clear") { serviceManager.runModelsClearCommand() }
            }
            commandButtonsRow(title: "Acceso") {
              Button("Allow") { serviceManager.runAllowCommand() }
              Button("Deny") { serviceManager.runDenyCommand() }
            }
            commandButtonsRow(title: "Resolver") {
              Button("Resolve + Persist") { serviceManager.runResolveCommand() }
            }

            Divider()
            Text("Resultado")
              .font(.caption)
              .foregroundStyle(.secondary)
            ScrollView {
              Text(serviceManager.commandOutput)
                .font(.system(.caption, design: .monospaced))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(8)
            }
            .frame(minHeight: 220)
            .background(Color.black.opacity(0.06))
            .clipShape(RoundedRectangle(cornerRadius: 8))
          }
          .padding(.vertical, 4)
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
              Button("Clear Logs") { serviceManager.clearLogs() }
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
      .padding(.top, 20)
      .padding(.horizontal, 16)
      .padding(.bottom, 16)
    }
    .alert("Aviso", isPresented: Binding(
      get: { serviceManager.alertMessage != nil },
      set: { value in if !value { serviceManager.alertMessage = nil } }
    )) {
      Button("OK", role: .cancel) {}
    } message: {
      Text(serviceManager.alertMessage ?? "")
    }
  }

  private func labeledField(_ title: String, placeholder: String, text: Binding<String>) -> some View {
    HStack {
      Text(title)
        .frame(width: 170, alignment: .leading)
      TextField(placeholder, text: text)
    }
  }

  private func labeledSecureField(_ title: String, placeholder: String, text: Binding<String>) -> some View {
    HStack {
      Text(title)
        .frame(width: 170, alignment: .leading)
      SecureField(placeholder, text: text)
    }
  }

  private func labeledNumberField(_ title: String, value: Binding<Int>) -> some View {
    HStack {
      Text(title)
        .frame(width: 170, alignment: .leading)
      TextField("0", value: value, format: .number)
    }
  }

  private func commandButtonsRow<Content: View>(title: String, @ViewBuilder content: () -> Content) -> some View {
    HStack(alignment: .top) {
      Text(title)
        .frame(width: 170, alignment: .leading)
        .foregroundStyle(.secondary)
      FlowLayout(spacing: 8) { content() }
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

private struct FlowLayout<Content: View>: View {
  let spacing: CGFloat
  let content: Content

  init(spacing: CGFloat = 8, @ViewBuilder content: () -> Content) {
    self.spacing = spacing
    self.content = content()
  }

  var body: some View {
    content
      .buttonStyle(.bordered)
      .controlSize(.small)
  }
}
