# Peekly

Peekly 是一个本地 HTTP 和 HTTPS 调试代理。桌面外壳使用 Wails v3.1.0，界面使用 React、TypeScript 和 daisyUI。

## 开始抓包

1. 启动应用并选择「开始抓包」→「使用系统代理」，首次启用时 macOS 可能请求管理员授权。
2. 在遵循系统代理的浏览器中刷新页面。手动连接模式下可单独为客户端配置 `127.0.0.1:9090`，也可使用下面的终端命令。
3. 在请求列表中选择一条流量，查看请求、标头、查询参数和响应。工具栏可将详情移到右侧，或移到底部并排查看请求和响应。
4. 停止抓包会恢复自动模式保存的原始设置。自行添加的系统代理需要手动撤销。

终端示例。Windows PowerShell 可将 `curl` 换成 `curl.exe`。

```sh
curl --proxy http://127.0.0.1:9090 http://example.com/
```

## 自动系统代理

在 macOS，自动模式使用 System Configuration 为当前活动网络服务配置 HTTP 和 HTTPS 代理。在 Windows，使用 WinINet 配置当前用户的 LAN 连接。开始时保存原配置，停止或正常退出时自动恢复。如果异常退出后系统仍指向 Peekly，重启 Peekly 会尝试恢复。系统要求重新授权时，可点击底栏的「恢复系统代理」。切换网络后可停止并重新开始捕获，以接入新的活动网络。

自动模式下，首选端口被占用时会自动使用系统分配的空闲端口。底栏和连接指引会显示当前实际端口。手动模式使用设定的端口。

自动模式只对支持系统代理的应用生效。使用独立代理配置或直接连接的应用、以及自带绕过规则的域名可能需要单独配置。自动设置代理不安装 HTTPS 根证书。此版本通过模拟系统设置验证了恢复和失败回滚，并在 macOS 验证了读取活动网络配置。实际系统写入和授权流程尚未做端到端验收。

## HTTPS 解密

在「HTTPS 解密」中导出 Peekly 专用根证书，并按对话框说明在客户端中信任。填入要调试的域名后应用规则，再重新建立客户端连接。支持精确域名、IP 和 `*.example.com` 形式的子域名匹配。未匹配的连接保持加密，只记录 CONNECT 元数据。证书私钥保留在当前用户的 Peekly 配置目录。

端口、HTTPS 解密规则和高级网络设置在当前运行期间有效。代理默认绑定回环地址，启用局域网接入时需要指定允许连接的客户端。抓包列表保留最近 500 条，每个方向的正文预览最多 64 KiB。WebSocket 每条记录保留最近 100 帧，每帧最多预览 1 KiB。需要留存时，可导出当前筛选结果为 HAR。勾选脱敏会省略正文、清理查询参数值及敏感标头，URL 路径仍保留。

## 桌面功能

- 九列实时流量表支持排序、主机导航、字段条件和正则筛选，以及保存筛选预设。详情可置于右侧，也可停靠底部并排显示请求和响应。
- 浅色、深色与跟随系统可在连接指引中选择。工具栏提供浅深色切换。
- 支持 HTTP、HTTPS、HTTP/2、WebSocket 帧和 SSE，并提供连接分阶段计时。支持查询参数、Cookies、标头、响应尾标、JSON 树、图片、Hex 和 Raw 预览。提供 GraphQL、JSON-RPC、JWT、multipart 和 gRPC 帧的基础查看。
- 支持新建请求、直接重发、编辑后重发、并排比较、复制为 cURL、保存正文预览，以及 HAR 导入和脱敏导出。
- 流量规则支持请求与响应断点、模拟响应、本地文件或目录映射、远程映射、标头修改、允许和阻止列表、绕过、禁用缓存、延迟和响应限速。
- 高级网络设置提供局域网客户端地址限制、DNS 映射、反向代理及 HTTP、HTTPS 或 SOCKS5 上游代理。

当前版本仍有功能差距。证书信任使用手动设置。自动系统代理适用于遵守系统设置的客户端。进程识别、透明抓包组件、下游 SOCKS 监听、自定义证书导入、Protobuf 描述文件解析、脚本扩展、MCP 和本地 AI 助手尚未交付。macOS 已在 Apple Silicon 上运行验证。Windows 便携包使用原生 WebView2，当前发布流程提供未做 Authenticode 签名的便携包。

对照资料可见 [Proxyman 官方文档](https://proxyman.com/docs/) 和 [Rockxy 官方文档](https://docs.rockxy.io/)。

## 从源码构建

需要 Go 1.26.10、Node.js 24，以及目标平台的 WebView。macOS 需要 Xcode 命令行工具，Windows 使用 Microsoft Edge WebView2 Runtime。

开发和测试的约定见[贡献指南](CONTRIBUTING.md)。可用以下命令分别构建本机安装包。

```sh
# macOS，接受 arm64、amd64 或 universal
./scripts/package-macos.sh
```

```powershell
# Windows，也可以指定 arm64
./scripts/package-windows.ps1 -Architecture amd64
```

## 打包与公证

`Package Peekly` 会在 PR、`main` 和版本标签上分别构建 macOS、Windows 的 x64 和 ARM64 包，并通过 `Package checks` 汇总结果。指向 `main` 上提交的 `vMAJOR.MINOR.PATCH` 标签会生成包含四个 ZIP 和 SHA-256 校验清单的 GitHub Release 草稿。

macOS 发布沿用 Relaydroid 当前的 Developer ID Application 团队 `VYG6HBHVJ9`。先统一前端、macOS 和 Windows 的版本，再创建版本标签。草稿就绪后，在配置了同一 Developer ID 和公证钥匙串配置的 Mac 上运行以下命令，将最后一个参数替换成 Relaydroid 使用的本机 profile 名称。

```sh
./scripts/publish-notarized-release.sh v0.2.1 VYG6HBHVJ9 RELAYDROID_NOTARYTOOL_PROFILE
```

脚本会校验草稿文件、启用 Hardened Runtime、签名并公证两个 macOS 包。两个票据均验证成功后，才更新安装包和校验清单并发布草稿。发布事件会再次验证安装包、签名团队与票据。证书私钥和公证凭据保存在本机钥匙串。

只公证本机构建的 macOS 包时可使用下列命令。

```sh
./scripts/notarize-macos.sh   build-out/Peekly-macOS-universal.zip build-out/Peekly-macOS-universal-notarized.zip   VYG6HBHVJ9 RELAYDROID_NOTARYTOOL_PROFILE
```

## 应用图标

使用内置 ImageGen，将第一张参考图中带眼睛的放大镜和眨眼表情合入第二张参考图的奶油色星星。保留橙色圆角底、星星和微笑，放大镜置于星星左眼，右侧保留眨眼，四周使用透明背景。源图位于 `build/appicon.png`，平台格式位于 `build/darwin/icons.icns` 和 `build/windows/icon.ico`。
