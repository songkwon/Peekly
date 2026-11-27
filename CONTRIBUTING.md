# 参与 Peekly

在 GitHub Issues 提交普通问题时，请附上 Peekly 版本、系统和芯片、抓包方式及复现步骤。安全问题请使用[私密漏洞报告](SECURITY.md)。

1. 从最新的 `main` 创建分支，仓库外的贡献者请先 fork。
2. 运行 `npm --prefix frontend ci` 和 `npm --prefix frontend run build`。在 macOS 运行 `go test -race ./...`，在 Windows 运行 `go test ./...`。
3. 修改 Go 代码时运行 `gofmt`。在发往 `main` 的 PR 中写明改动原因及实际验证的平台。
4. 四个平台的 `Package checks` 通过后，由维护者评审并 squash 合并。

请在提交截图、日志或 HAR 文件前移除访问令牌、Cookie、敏感 URL 和真实请求正文。

## 从源码构建

需要 Go 1.26.10 和 Node.js 24。macOS 使用 Xcode 命令行工具，Windows 使用 Microsoft Edge WebView2 Runtime。

- macOS 运行 `./scripts/package-macos.sh`，也可指定 `arm64`、`amd64` 或 `universal`。
- Windows 运行 `./scripts/package-windows.ps1 -Architecture amd64`，也可指定 `arm64`。

## 发布与 Homebrew

1. 同步前端、macOS 和 Windows 的版本。在 `main` 的发布提交上创建并推送 `vMAJOR.MINOR.PATCH` 标签。打包流水线会生成包含四个 ZIP 和 SHA-256 清单的 Release 草稿。
2. 在配置了 Relaydroid 公证钥匙串的 Mac 上运行 `./scripts/publish-notarized-release.sh <标签> VYG6HBHVJ9 <公证配置名称>`。脚本验证草稿，为两个 macOS 包签名并公证，再上传安装包和校验清单，发布 Release。
3. 脚本会用已发布、通过公证校验的两个 macOS 包生成 Homebrew 更新 PR。待 `Package checks` 通过后 squash 合并。如果 Release 已发布而 Homebrew PR 创建失败，可用 `./scripts/open-cask-pr.sh <标签>` 重试。
