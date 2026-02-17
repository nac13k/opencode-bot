// swift-tools-version: 5.10
import PackageDescription

let package = Package(
  name: "opencode-bot",
  platforms: [
    .macOS(.v14),
  ],
  products: [
    .executable(
      name: "OpencodeBot",
      targets: ["OpencodeBot"]
    ),
  ],
  targets: [
    .executableTarget(
      name: "OpencodeBot",
      path: "Sources",
      linkerSettings: [
        .linkedLibrary("sqlite3"),
      ]
    ),
  ]
)
