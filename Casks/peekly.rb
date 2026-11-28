cask "peekly" do
  arch arm: "arm64", intel: "amd64"

  version "0.3.1"
  sha256 arm:   "ba5a08259dc68dff3a78535031c5eab30ef12173ec19668cb8a27a6edb164aa7",
         intel: "d7a034b08826a847fea9838b563267575316bdf30358ba51bf44f3704780ea45"

  url "https://github.com/songkwon/Peekly/releases/download/v#{version}/Peekly-macOS-#{arch}.zip"
  name "Peekly"
  desc "Capture and debug HTTP and HTTPS traffic"
  homepage "https://github.com/songkwon/Peekly"

  depends_on macos: :monterey

  app "Peekly.app"
end
