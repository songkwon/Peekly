cask "peekly" do
  arch arm: "arm64", intel: "amd64"

  version "0.2.2"
  sha256 arm:   "0fc1de8f54e19c6b4bc79836c946b5f52ae9f8a906d0463b9b57f83c6bbdfb8c",
         intel: "d72bed770329231b7c2a7d56f4dd955c8aa1389a41486809095e416c8fe5f0eb"

  url "https://github.com/songkwon/Peekly/releases/download/v#{version}/Peekly-macOS-#{arch}.zip"
  name "Peekly"
  desc "Capture and debug HTTP and HTTPS traffic"
  homepage "https://github.com/songkwon/Peekly"

  depends_on macos: :monterey

  app "Peekly.app"
end
