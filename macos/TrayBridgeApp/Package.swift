// swift-tools-version: 5.10
import PackageDescription

let package = Package(
  name: "TrayBridgeApp",
  platforms: [
    .macOS(.v14),
  ],
  products: [
    .executable(
      name: "TrayBridgeApp",
      targets: ["TrayBridgeApp"]
    ),
  ],
  targets: [
    .executableTarget(
      name: "TrayBridgeApp",
      path: "Sources"
    ),
  ]
)
