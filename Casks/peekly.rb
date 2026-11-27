cask "peekly" do
  arch arm: "arm64", intel: "amd64"

  version "0.3.0"
  sha256 arm:   "3395169436d81b19c34debfcfd5b651bbc401865d46c3d7d95ee0820880468b4",
         intel: "6c3c900a3a31531bd14d51c6a4318fe659be1dde3b6143cb94bf2c4846d6b095"

  url "https://github.com/songkwon/Peekly/releases/download/v#{version}/Peekly-macOS-#{arch}.zip"
  name "Peekly"
  desc "Capture and debug HTTP and HTTPS traffic"
  homepage "https://github.com/songkwon/Peekly"

  depends_on macos: :monterey

  app "Peekly.app"
end
