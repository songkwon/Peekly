# 参与 Peekly

在 GitHub Issues 提交普通问题时，请附上 Peekly 版本、系统和芯片、抓包方式及复现步骤。安全问题请使用[私密漏洞报告](SECURITY.md)。

1. 从最新的 `main` 创建分支，仓库外的贡献者请先 fork。
2. 运行 `npm --prefix frontend ci` 和 `npm --prefix frontend run build`。在 macOS 运行 `go test -race ./...`，在 Windows 运行 `go test ./...`。
3. 修改 Go 代码时运行 `gofmt`。在发往 `main` 的 PR 中写明改动原因及实际验证的平台。
4. 四个平台的 `Package checks` 通过后，由维护者评审并 squash 合并。

请在提交截图、日志或 HAR 文件前移除访问令牌、Cookie、敏感 URL 和真实请求正文。
