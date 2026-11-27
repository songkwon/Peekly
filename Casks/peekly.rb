cask "peekly" do
  arch arm: "arm64", intel: "amd64"

  version "0.2.1"
  sha256 arm:   "c2d6751388e056026ad7c409a07f198e9ffcccd61bb6a856058a4ede7f9945bc",
         intel: "18590e8f23aa57e051116aeedf17f03a473031140f143e48b30c11db9324d995"

  url "https://github.com/songkwon/Peekly/releases/download/v#{version}/Peekly-macOS-#{arch}.zip"
  name "Peekly"
  desc "Capture and debug HTTP and HTTPS traffic"
  homepage "https://github.com/songkwon/Peekly"

  depends_on macos: :monterey

  app "Peekly.app"
end
