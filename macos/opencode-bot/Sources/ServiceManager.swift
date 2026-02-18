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

enum SessionsSource: String, CaseIterable, Identifiable {
  case endpoint
  case cli
  case both

  var id: String { rawValue }
}

final class ServiceManager: ObservableObject {
  @Published var isRunning = false
  @Published var statusText = "Ready"
  @Published var logs = ""
  @Published var commandOutput = "Sin resultados todavia"
  @Published var openCodeStatusText = "No verificado"
  @Published var openCodeManagedRunning = false

  @Published var botToken = ""
  @Published var adminUserIds = ""
  @Published var allowedUserIds = ""
  @Published var botTransport: BotTransport = .polling
  @Published var webhookUrl = ""
  @Published var webhookListenAddr = ":8090"
  @Published var botPollingIntervalSeconds = 2
  @Published var dataDir = "./data"
  @Published var opencodeTimeoutMs = 120000
  @Published var opencodeServerUrl = "http://127.0.0.1:4096"
  @Published var opencodeServerUsername = "opencode"
  @Published var opencodeServerPassword = ""
  @Published var opencodeBinaryPath = ""
  @Published var opencodeCLIWorkDir = ""
  @Published var defaultSessionId = ""
  @Published var controlWebServer = false
  @Published var controlSocketPath = "/tmp/opencode-bot.sock"
  @Published var healthPort = 4097
  @Published var relayMode: RelayMode = .last
  @Published var relayFallback = true
  @Published var relayFallbackDelayMs = 3000
  @Published var relaySSEEnabled = false
  @Published var sessionsListLimit = 5
  @Published var sessionsSource: SessionsSource = .both
  @Published var sessionsShowIDList = true
  @Published var usingBundledServer = false
  @Published var alertMessage: String? = nil

  @Published var commandChatId = ""
  @Published var commandUserId = ""
  @Published var commandTargetUserId = ""
  @Published var commandSessionId = ""
  @Published var commandModelId = ""
  @Published var commandUsernames = ""

  private var process: Process?
  private var openCodeProcess: Process?
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
      botPollingIntervalSeconds = Int(values["BOT_POLLING_INTERVAL_SECONDS"] ?? "2") ?? 2
      dataDir = values["DATA_DIR"] ?? "./data"
      opencodeTimeoutMs = Int(values["OPENCODE_TIMEOUT_MS"] ?? "120000") ?? 120000
      opencodeServerUrl = values["OPENCODE_SERVER_URL"] ?? "http://127.0.0.1:4096"
      opencodeServerUsername = values["OPENCODE_SERVER_USERNAME"] ?? "opencode"
      opencodeServerPassword = values["OPENCODE_SERVER_PASSWORD"] ?? ""
      opencodeBinaryPath = values["OPENCODE_BINARY"] ?? ""
      opencodeCLIWorkDir = values["OPENCODE_CLI_WORKDIR"] ?? ""
      defaultSessionId = values["DEFAULT_SESSION_ID"] ?? ""
      controlWebServer = (values["CONTROL_WEB_SERVER"] ?? "false").lowercased() == "true"
      controlSocketPath = values["CONTROL_SOCKET_PATH"] ?? "/tmp/opencode-bot.sock"
      healthPort = Int(values["HEALTH_PORT"] ?? "4097") ?? 4097
      relayMode = RelayMode(rawValue: values["RELAY_MODE"] ?? "last") ?? .last
      relayFallback = (values["RELAY_FALLBACK"] ?? "true").lowercased() != "false"
      relayFallbackDelayMs = Int(values["RELAY_FALLBACK_DELAY_MS"] ?? "3000") ?? 3000
      relaySSEEnabled = (values["RELAY_SSE_ENABLED"] ?? "false").lowercased() == "true"
      sessionsListLimit = Int(values["SESSIONS_LIST_LIMIT"] ?? "5") ?? 5
      sessionsSource = SessionsSource(rawValue: values["SESSIONS_SOURCE"] ?? "both") ?? .both
      sessionsShowIDList = (values["SESSIONS_SHOW_ID_LIST"] ?? "true").lowercased() != "false"
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

    let runtimeDir = ensureRuntimeDirectory()
    let proc = Process()
    proc.currentDirectoryURL = runtimeDir
    proc.executableURL = bridgeBinary
    proc.arguments = ["serve"]

    var env = ProcessInfo.processInfo.environment
    env["BOT_TOKEN"] = botToken
    env["ADMIN_USER_IDS"] = adminUserIds
    env["ALLOWED_USER_IDS"] = allowedUserIds
    env["BOT_TRANSPORT"] = botTransport.rawValue
    env["WEBHOOK_URL"] = webhookUrl
    env["WEBHOOK_LISTEN_ADDR"] = webhookListenAddr
    env["BOT_POLLING_INTERVAL_SECONDS"] = String(max(1, botPollingIntervalSeconds))
    env["DATA_DIR"] = resolvedDataDir(base: runtimeDir)
    env["OPENCODE_TIMEOUT_MS"] = String(opencodeTimeoutMs)
    env["OPENCODE_SERVER_URL"] = opencodeServerUrl
    env["OPENCODE_SERVER_USERNAME"] = opencodeServerUsername
    if !opencodeBinaryPath.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["OPENCODE_BINARY"] = opencodeBinaryPath
    }
    if !opencodeCLIWorkDir.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["OPENCODE_CLI_WORKDIR"] = opencodeCLIWorkDir
    }
    env["HEALTH_PORT"] = String(healthPort)
    env["RELAY_MODE"] = relayMode.rawValue
    env["RELAY_FALLBACK"] = relayFallback ? "true" : "false"
    env["RELAY_FALLBACK_DELAY_MS"] = String(relayFallbackDelayMs)
    env["RELAY_SSE_ENABLED"] = relaySSEEnabled ? "true" : "false"
    env["SESSIONS_LIST_LIMIT"] = String(max(1, sessionsListLimit))
    env["SESSIONS_SOURCE"] = sessionsSource.rawValue
    env["SESSIONS_SHOW_ID_LIST"] = sessionsShowIDList ? "true" : "false"
    if !opencodeServerPassword.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
      env["OPENCODE_SERVER_PASSWORD"] = opencodeServerPassword
    }
    if !defaultSessionId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
    env["DEFAULT_SESSION_ID"] = defaultSessionId
    }
    env["CONTROL_WEB_SERVER"] = controlWebServer ? "true" : "false"
    env["CONTROL_SOCKET_PATH"] = controlSocketPath
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

  func checkOpenCodeServer() {
    guard let healthURL = openCodeHealthURL() else {
      openCodeStatusText = "URL invalida"
      return
    }

    var request = URLRequest(url: healthURL)
    request.httpMethod = "GET"
    request.timeoutInterval = 8
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue(basicAuthHeader(username: opencodeServerUsername, password: opencodeServerPassword), forHTTPHeaderField: "Authorization")

    URLSession.shared.dataTask(with: request) { [weak self] data, response, error in
      DispatchQueue.main.async {
        if let error {
          self?.openCodeStatusText = "Caido: \(error.localizedDescription)"
          self?.appendLog("[warn] OpenCode health failed: \(error.localizedDescription)")
          return
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if status >= 200 && status < 300 {
          self?.openCodeStatusText = "OK (HTTP \(status))"
          self?.appendLog("[info] OpenCode health OK")
          return
        }
        let body = data.flatMap { String(data: $0, encoding: .utf8) } ?? ""
        self?.openCodeStatusText = "Error HTTP \(status)"
        self?.appendLog("[warn] OpenCode health HTTP \(status): \(body)")
      }
    }.resume()
  }

  func startOpenCodeServer() {
    if let openCodeProcess, openCodeProcess.isRunning {
      openCodeStatusText = "Ya esta corriendo (gestionado)"
      return
    }

    guard let opencodeBinary = resolveOpenCodeBinary() else {
      openCodeManagedRunning = false
      openCodeStatusText = "No se encontro binario opencode"
      appendLog("[error] opencode binary not found. Set OPENCODE_BINARY in settings.")
      return
    }

    let runtimeDir = ensureRuntimeDirectory()
    let proc = Process()
    proc.currentDirectoryURL = runtimeDir
    proc.executableURL = opencodeBinary
    proc.arguments = ["serve"]

    let outputPipe = Pipe()
    proc.standardOutput = outputPipe
    proc.standardError = outputPipe
    outputPipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
      let data = handle.availableData
      if data.isEmpty { return }
      let chunk = String(decoding: data, as: UTF8.self)
      DispatchQueue.main.async {
        self?.appendLog("[opencode] \(chunk.trimmingCharacters(in: .whitespacesAndNewlines))")
      }
    }

    proc.terminationHandler = { [weak self] process in
      DispatchQueue.main.async {
        self?.openCodeManagedRunning = false
        self?.openCodeProcess = nil
        self?.openCodeStatusText = "Detenido (exit \(process.terminationStatus))"
        self?.appendLog("[info] opencode serve stopped with status \(process.terminationStatus)")
      }
    }

    do {
      try proc.run()
      openCodeProcess = proc
      openCodeManagedRunning = true
      openCodeStatusText = "Iniciado (gestionado por app)"
      appendLog("[info] Started opencode serve at \(opencodeBinary.path)")
    } catch {
      openCodeManagedRunning = false
      openCodeStatusText = "No se pudo iniciar"
      appendLog("[error] Failed to start opencode serve: \(error.localizedDescription)")
    }
  }

  func stopOpenCodeServer() {
    guard let openCodeProcess else {
      openCodeStatusText = "No gestionado por app"
      return
    }
    if openCodeProcess.isRunning {
      openCodeProcess.terminate()
      openCodeStatusText = "Deteniendo..."
      appendLog("[info] Stopping opencode serve")
    }
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
    if !controlWebServer {
      sendUnixSocketRequest(path: path, method: method, body: body, fallbackToWeb: true)
      return
    }

    sendWebRequest(path: path, method: method, body: body)
  }

  private func sendWebRequest(path: String, method: String, body: Data?) {
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

  private func sendUnixSocketRequest(path: String, method: String, body: Data?, fallbackToWeb: Bool) {
    let socket = controlSocketPath.trimmingCharacters(in: .whitespacesAndNewlines)
    if socket.isEmpty {
      commandOutput = "CONTROL_SOCKET_PATH vacio"
      return
    }

    let curl = Process()
    curl.executableURL = URL(fileURLWithPath: "/usr/bin/curl")
    var args = ["--silent", "--show-error", "--write-out", "\n__HTTP_STATUS__:%{http_code}", "--unix-socket", socket, "-X", method, "http://localhost\(path)"]
    if let body {
      args.append(contentsOf: ["-H", "Content-Type: application/json", "--data-binary", String(decoding: body, as: UTF8.self)])
    }
    curl.arguments = args

    let outputPipe = Pipe()
    let errorPipe = Pipe()
    curl.standardOutput = outputPipe
    curl.standardError = errorPipe

    DispatchQueue.global(qos: .userInitiated).async { [weak self] in
      do {
        try curl.run()
      } catch {
        DispatchQueue.main.async {
          self?.commandOutput = "Error ejecutando curl: \(error.localizedDescription)"
          self?.appendLog("[error] Curl unix request failed: \(error.localizedDescription)")
        }
        return
      }

      curl.waitUntilExit()
      let outData = outputPipe.fileHandleForReading.readDataToEndOfFile()
      let errData = errorPipe.fileHandleForReading.readDataToEndOfFile()
      let outText = String(decoding: outData, as: UTF8.self)
      let errText = String(decoding: errData, as: UTF8.self)

      DispatchQueue.main.async {
        if curl.terminationStatus != 0 {
          if fallbackToWeb {
            self?.appendLog("[warn] Unix socket request failed, trying TCP fallback on 127.0.0.1:\(self?.healthPort ?? 0)")
            self?.sendWebRequest(path: path, method: method, body: body)
            return
          }
          self?.commandOutput = "Error: \(errText.isEmpty ? outText : errText)"
          self?.appendLog("[error] Command \(path) over unix socket failed: \(errText)")
          return
        }

        let marker = "\n__HTTP_STATUS__:"
        let parts = outText.components(separatedBy: marker)
        let payloadText = parts.first ?? ""
        let httpStatus = parts.count > 1 ? parts.last?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "0" : "0"
        let pretty = self?.prettyJSON(payloadText) ?? payloadText
        self?.commandOutput = "HTTP \(httpStatus)\n\n\(pretty.isEmpty ? "(sin body)" : pretty)"
        self?.appendLog("[info] Command \(path) -> HTTP \(httpStatus) (unix)")
      }
    }
  }

  private func prettyJSON(_ text: String) -> String {
    let data = Data(text.utf8)
    guard let object = try? JSONSerialization.jsonObject(with: data),
          let pretty = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
          let prettyText = String(data: pretty, encoding: .utf8)
    else {
      return text.trimmingCharacters(in: .whitespacesAndNewlines)
    }
    return prettyText
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
      "BOT_POLLING_INTERVAL_SECONDS": String(max(1, botPollingIntervalSeconds)),
      "DATA_DIR": dataDir,
      "OPENCODE_TIMEOUT_MS": String(opencodeTimeoutMs),
      "OPENCODE_SERVER_URL": opencodeServerUrl,
      "OPENCODE_SERVER_USERNAME": opencodeServerUsername,
      "OPENCODE_SERVER_PASSWORD": opencodeServerPassword,
      "OPENCODE_BINARY": opencodeBinaryPath,
      "OPENCODE_CLI_WORKDIR": opencodeCLIWorkDir,
      "DEFAULT_SESSION_ID": defaultSessionId,
      "CONTROL_WEB_SERVER": controlWebServer ? "true" : "false",
      "CONTROL_SOCKET_PATH": controlSocketPath,
      "HEALTH_PORT": String(healthPort),
      "RELAY_MODE": relayMode.rawValue,
      "RELAY_FALLBACK": relayFallback ? "true" : "false",
      "RELAY_FALLBACK_DELAY_MS": String(relayFallbackDelayMs),
      "RELAY_SSE_ENABLED": relaySSEEnabled ? "true" : "false",
      "SESSIONS_LIST_LIMIT": String(max(1, sessionsListLimit)),
      "SESSIONS_SOURCE": sessionsSource.rawValue,
      "SESSIONS_SHOW_ID_LIST": sessionsShowIDList ? "true" : "false",
    ]
  }

  private func ensureRuntimeDirectory() -> URL {
    let fm = FileManager.default
    let base: URL
    do {
      let appSupport = try fm.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
      base = appSupport.appendingPathComponent("opencode-bot", isDirectory: true).appendingPathComponent("runtime", isDirectory: true)
      try fm.createDirectory(at: base, withIntermediateDirectories: true)
      return base
    } catch {
      appendLog("[warn] Failed to prepare runtime dir in Application Support: \(error.localizedDescription)")
      let temp = fm.temporaryDirectory.appendingPathComponent("opencode-bot-runtime", isDirectory: true)
      try? fm.createDirectory(at: temp, withIntermediateDirectories: true)
      return temp
    }
  }

  private func resolvedDataDir(base: URL) -> String {
    let raw = dataDir.trimmingCharacters(in: .whitespacesAndNewlines)
    if raw.isEmpty {
      return base.appendingPathComponent("data", isDirectory: true).path
    }
    if raw.hasPrefix("/") {
      return raw
    }
    return base.appendingPathComponent(raw, isDirectory: true).path
  }

  private func openCodeHealthURL() -> URL? {
    let base = opencodeServerUrl.trimmingCharacters(in: .whitespacesAndNewlines)
    if base.isEmpty { return nil }
    let suffix = base.hasSuffix("/") ? "global/health" : "/global/health"
    return URL(string: base + suffix)
  }

  private func basicAuthHeader(username: String, password: String) -> String {
    let raw = "\(username):\(password)"
    let encoded = Data(raw.utf8).base64EncodedString()
    return "Basic \(encoded)"
  }

  private func resolveOpenCodeBinary() -> URL? {
    let explicit = opencodeBinaryPath.trimmingCharacters(in: .whitespacesAndNewlines)
    if !explicit.isEmpty {
      let url = URL(fileURLWithPath: explicit)
      if FileManager.default.isExecutableFile(atPath: url.path) {
        return url
      }
    }

    let candidates = [
      "/opt/homebrew/bin/opencode",
      "/usr/local/bin/opencode",
      "/usr/bin/opencode",
    ]
    for candidate in candidates where FileManager.default.isExecutableFile(atPath: candidate) {
      return URL(fileURLWithPath: candidate)
    }

    let shell = Process()
    shell.executableURL = URL(fileURLWithPath: "/usr/bin/which")
    shell.arguments = ["opencode"]
    let out = Pipe()
    shell.standardOutput = out
    shell.standardError = Pipe()
    do {
      try shell.run()
      shell.waitUntilExit()
      if shell.terminationStatus == 0 {
        let text = String(decoding: out.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
          .trimmingCharacters(in: .whitespacesAndNewlines)
        if !text.isEmpty, FileManager.default.isExecutableFile(atPath: text) {
          return URL(fileURLWithPath: text)
        }
      }
    } catch {
      return nil
    }
    return nil
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
