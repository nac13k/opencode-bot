import Foundation

enum BotTransport: String, CaseIterable, Identifiable {
  case polling
  case webhook

  var id: String { rawValue }
}

enum RelayMode: String, CaseIterable, Identifiable {
  case last
  case final

  var id: String { rawValue }
}

final class ServiceManager: ObservableObject {
  @Published var isRunning = false
  @Published var statusText = "Ready"
  @Published var logs = ""
  @Published var commandOutput = "Sin resultados todavia"

  @Published var botToken = ""
  @Published var adminUserIds = ""
  @Published var allowedUserIds = ""
  @Published var botTransport: BotTransport = .polling
  @Published var webhookUrl = ""
  @Published var webhookListenAddr = ":8090"
  @Published var dataDir = "./data"
  @Published var opencodeTimeoutMs = 120000
  @Published var opencodeServerUrl = "http://127.0.0.1:4096"
  @Published var opencodeServerUsername = "opencode"
  @Published var opencodeServerPassword = ""
  @Published var defaultSessionId = ""
  @Published var healthPort = 4097
  @Published var relayMode: RelayMode = .last
  @Published var relayFallback = true
  @Published var relayFallbackDelayMs = 3000
  @Published var usingBundledServer = false
  @Published var alertMessage: String? = nil

  @Published var commandChatId = ""
  @Published var commandUserId = ""
  @Published var commandTargetUserId = ""
  @Published var commandSessionId = ""
  @Published var commandModelId = ""
  @Published var commandUsernames = ""

  private var process: Process?
  private var store: SQLiteConfigStore? = nil

  init() {
    var loadedStore: SQLiteConfigStore?
    do {
      let candidate = try SQLiteConfigStore()
      loadedStore = candidate
      let values = try candidate.loadSettings()
      botToken = values["BOT_TOKEN"] ?? ""
      adminUserIds = values["ADMIN_USER_IDS"] ?? ""
      allowedUserIds = values["ALLOWED_USER_IDS"] ?? ""
      botTransport = BotTransport(rawValue: values["BOT_TRANSPORT"] ?? "polling") ?? .polling
      webhookUrl = values["WEBHOOK_URL"] ?? ""
      webhookListenAddr = values["WEBHOOK_LISTEN_ADDR"] ?? ":8090"
      dataDir = values["DATA_DIR"] ?? "./data"
      opencodeTimeoutMs = Int(values["OPENCODE_TIMEOUT_MS"] ?? "120000") ?? 120000
      opencodeServerUrl = values["OPENCODE_SERVER_URL"] ?? "http://127.0.0.1:4096"
      opencodeServerUsername = values["OPENCODE_SERVER_USERNAME"] ?? "opencode"
      opencodeServerPassword = values["OPENCODE_SERVER_PASSWORD"] ?? ""
      defaultSessionId = values["DEFAULT_SESSION_ID"] ?? ""
      healthPort = Int(values["HEALTH_PORT"] ?? "4097") ?? 4097
      relayMode = RelayMode(rawValue: values["RELAY_MODE"] ?? "last") ?? .last
      relayFallback = (values["RELAY_FALLBACK"] ?? "true").lowercased() != "false"
      relayFallbackDelayMs = Int(values["RELAY_FALLBACK_DELAY_MS"] ?? "3000") ?? 3000
    } catch {
      statusText = "SQLite config unavailable"
      appendLog("[error] Failed to load SQLite config: \(error.localizedDescription)")
    }
    store = loadedStore
  }

  func saveConfig() {
    guard let store else {
      statusText = "SQLite config unavailable"
      appendLog("[error] Save failed: SQLite store not ready")
      return
    }
    do {
      try store.saveSettings(currentConfigValues())
      statusText = "Configuration saved"
      appendLog("[info] Saved configuration to SQLite")
    } catch {
      statusText = "Failed to save configuration"
      appendLog("[error] Save failed: \(error.localizedDescription)")
    }
  }

  func startService() {
    if isRunning {
      statusText = "Service is already running"
      return
    }

    guard let serverDir = resolveBundledServerDirectory() else {
      usingBundledServer = false
      statusText = "Bundled server not found"
      alertMessage = "La app no incluye el payload Go embebido. Recompila el bundle."
      appendLog("[error] App bundle does not include embedded bridge binary")
      return
    }
    guard let bridgeBinary = resolveBundledBridgeBinary(serverDir: serverDir) else {
      usingBundledServer = false
      statusText = "Bridge binary missing"
      alertMessage = "No se encontro el binario 'bridge' en el servidor embebido."
      appendLog("[error] Missing embedded bridge binary")
      return
    }

    if botToken.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      statusText = "BOT_TOKEN is required"
      appendLog("[error] BOT_TOKEN is required")
      return
    }
    if adminUserIds.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      statusText = "ADMIN_USER_IDS is required"
      appendLog("[error] ADMIN_USER_IDS is required")
      return
    }
    if botTransport == .webhook && webhookUrl.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      statusText = "WEBHOOK_URL is required"
      appendLog("[error] WEBHOOK_URL is required for webhook mode")
      return
    }

    saveConfig()

    let proc = Process()
    proc.currentDirectoryURL = serverDir
    proc.executableURL = bridgeBinary
    proc.arguments = ["serve"]

    var env = ProcessInfo.processInfo.environment
    env["BOT_TOKEN"] = botToken
    env["ADMIN_USER_IDS"] = adminUserIds
    env["ALLOWED_USER_IDS"] = allowedUserIds
    env["BOT_TRANSPORT"] = botTransport.rawValue
    env["WEBHOOK_URL"] = webhookUrl
    env["WEBHOOK_LISTEN_ADDR"] = webhookListenAddr
    env["DATA_DIR"] = dataDir
    env["OPENCODE_TIMEOUT_MS"] = String(opencodeTimeoutMs)
    env["OPENCODE_SERVER_URL"] = opencodeServerUrl
    env["OPENCODE_SERVER_USERNAME"] = opencodeServerUsername
    env["HEALTH_PORT"] = String(healthPort)
    env["RELAY_MODE"] = relayMode.rawValue
    env["RELAY_FALLBACK"] = relayFallback ? "true" : "false"
    env["RELAY_FALLBACK_DELAY_MS"] = String(relayFallbackDelayMs)
    if !opencodeServerPassword.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["OPENCODE_SERVER_PASSWORD"] = opencodeServerPassword
    }
    if !defaultSessionId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["DEFAULT_SESSION_ID"] = defaultSessionId
    }
    proc.environment = env

    let outputPipe = Pipe()
    proc.standardOutput = outputPipe
    proc.standardError = outputPipe
    outputPipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
      let data = handle.availableData
      if data.isEmpty { return }
      let chunk = String(decoding: data, as: UTF8.self)
      DispatchQueue.main.async {
        self?.appendLog(chunk.trimmingCharacters(in: .newlines))
      }
    }

    proc.terminationHandler = { [weak self] process in
      DispatchQueue.main.async {
        self?.isRunning = false
        self?.process = nil
        self?.statusText = "Service stopped (exit \(process.terminationStatus))"
        self?.appendLog("[info] Service stopped with status \(process.terminationStatus)")
      }
    }

    do {
      try proc.run()
      process = proc
      isRunning = true
      usingBundledServer = true
      statusText = "Service running"
      appendLog("[info] Started Go bridge at \(bridgeBinary.path)")
    } catch {
      statusText = "Failed to start service"
      appendLog("[error] Failed to start: \(error.localizedDescription)")
    }
  }

  func stopService() {
    guard let process else {
      statusText = "Service is not running"
      return
    }
    if process.isRunning {
      process.terminate()
      statusText = "Stopping service"
      appendLog("[info] Stopping service")
    }
  }

  func restartService() {
    if isRunning {
      stopService()
      DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) { [weak self] in
        self?.startService()
      }
      return
    }
    startService()
  }

  func clearLogs() {
    logs = ""
  }

  func runStatusCommand() { runCommand(path: "/command/status", payload: baseChatUserPayload()) }
  func runSessionCurrentCommand() { runCommand(path: "/command/session/get", payload: baseChatUserPayload()) }
  func runSessionListCommand() { runCommand(path: "/command/session/list", payload: baseChatUserPayload()) }
  func runSessionUseCommand() {
    var payload = baseChatUserPayload()
    payload["sessionId"] = commandSessionId
    runCommand(path: "/command/session/use", payload: payload)
  }
  func runSessionNewCommand() { runCommand(path: "/command/session/new", payload: baseChatUserPayload()) }
  func runModelsListCommand() { runCommand(path: "/command/models/list", payload: baseChatUserPayload()) }
  func runModelsSetCommand() {
    var payload = baseChatUserPayload()
    payload["model"] = commandModelId
    runCommand(path: "/command/models/set", payload: payload)
  }
  func runModelsClearCommand() { runCommand(path: "/command/models/clear", payload: baseChatUserPayload()) }
  func runCompactCommand() { runCommand(path: "/command/compact", payload: baseChatUserPayload()) }
  func runAllowCommand() { runCommand(path: "/command/allow", payload: ["targetUserId": commandTargetUserId]) }
  func runDenyCommand() { runCommand(path: "/command/deny", payload: ["targetUserId": commandTargetUserId]) }
  func runListAccessCommand() { runGetCommand(path: "/command/access/list") }
  func runResolveCommand() {
    let usernames = commandUsernames
      .split { $0 == "," || $0 == " " || $0 == "\n" || $0 == "\t" }
      .map { String($0) }
    runCommand(path: "/resolve", payload: ["usernames": usernames])
  }

  private func runCommand(path: String, payload: [String: Any]) {
    guard let body = try? JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys]) else {
      commandOutput = "No se pudo serializar payload"
      return
    }
    sendRequest(path: path, method: "POST", body: body)
  }

  private func runGetCommand(path: String) {
    sendRequest(path: path, method: "GET", body: nil)
  }

  private func sendRequest(path: String, method: String, body: Data?) {
    guard let url = URL(string: "http://127.0.0.1:\(healthPort)\(path)") else {
      commandOutput = "URL invalida"
      return
    }

    var request = URLRequest(url: url)
    request.httpMethod = method
    request.timeoutInterval = 30
    if let body {
      request.httpBody = body
      request.setValue("application/json", forHTTPHeaderField: "Content-Type")
    }

    let task = URLSession.shared.dataTask(with: request) { [weak self] data, response, error in
      DispatchQueue.main.async {
        if let error {
          self?.commandOutput = "Error: \(error.localizedDescription)"
          self?.appendLog("[error] Command \(path) failed: \(error.localizedDescription)")
          return
        }

        let httpStatus = (response as? HTTPURLResponse)?.statusCode ?? 0
        let payloadText: String
        if let data, !data.isEmpty,
           let object = try? JSONSerialization.jsonObject(with: data),
           let pretty = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
           let prettyText = String(data: pretty, encoding: .utf8)
        {
          payloadText = prettyText
        } else if let data, !data.isEmpty {
          payloadText = String(decoding: data, as: UTF8.self)
        } else {
          payloadText = "(sin body)"
        }

        self?.commandOutput = "HTTP \(httpStatus)\n\n\(payloadText)"
        self?.appendLog("[info] Command \(path) -> HTTP \(httpStatus)")
      }
    }
    task.resume()
  }

  private func baseChatUserPayload() -> [String: Any] {
    [
      "chatId": commandChatId,
      "userId": commandUserId,
    ]
  }

  private func currentConfigValues() -> [String: String] {
    [
      "BOT_TOKEN": botToken,
      "ADMIN_USER_IDS": adminUserIds,
      "ALLOWED_USER_IDS": allowedUserIds,
      "BOT_TRANSPORT": botTransport.rawValue,
      "WEBHOOK_URL": webhookUrl,
      "WEBHOOK_LISTEN_ADDR": webhookListenAddr,
      "DATA_DIR": dataDir,
      "OPENCODE_TIMEOUT_MS": String(opencodeTimeoutMs),
      "OPENCODE_SERVER_URL": opencodeServerUrl,
      "OPENCODE_SERVER_USERNAME": opencodeServerUsername,
      "OPENCODE_SERVER_PASSWORD": opencodeServerPassword,
      "DEFAULT_SESSION_ID": defaultSessionId,
      "HEALTH_PORT": String(healthPort),
      "RELAY_MODE": relayMode.rawValue,
      "RELAY_FALLBACK": relayFallback ? "true" : "false",
      "RELAY_FALLBACK_DELAY_MS": String(relayFallbackDelayMs),
    ]
  }

  private func resolveBundledServerDirectory() -> URL? {
    guard let resources = Bundle.main.resourceURL else { return nil }
    let server = resources.appendingPathComponent("server", isDirectory: true)
    var isDirectory: ObjCBool = false
    guard FileManager.default.fileExists(atPath: server.path, isDirectory: &isDirectory), isDirectory.boolValue else {
      return nil
    }
    return server
  }

  private func resolveBundledBridgeBinary(serverDir: URL) -> URL? {
    let binary = serverDir.appendingPathComponent("bridge")
    guard FileManager.default.fileExists(atPath: binary.path) else { return nil }
    return binary
  }

  private func appendLog(_ line: String) {
    guard !line.isEmpty else { return }
    let timestamp = ISO8601DateFormatter().string(from: Date())
    logs += "[\(timestamp)] \(line)\n"
    if logs.count > 80_000 {
      logs = String(logs.suffix(80_000))
    }
  }
}
