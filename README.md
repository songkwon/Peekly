# Peekly

Peekly 是一款适用于 macOS 和 Windows 的 HTTP/HTTPS 抓包与调试工具。

## 安装

macOS 可通过 Homebrew 安装。安装包已签名并通过 Apple 公证，Homebrew 会自动选择 Apple Silicon 或 Intel 版本。

```sh
brew tap songkwon/peekly https://github.com/songkwon/Peekly.git
brew install --cask songkwon/peekly/peekly
```

Windows 或手动安装请前往 [GitHub Releases](https://github.com/songkwon/Peekly/releases/latest)，下载对应系统和芯片的 ZIP。

## 开始抓包

1. 打开 Peekly，选择「开始抓包」→「使用系统代理」。
2. 在使用系统代理的浏览器中刷新页面，点击流量列表里的请求查看详情。手动连接地址见应用底栏或「连接指引」。
3. 如需查看 HTTPS 正文，在「HTTPS 解密」中按提示信任根证书，并添加要调试的域名。

源码构建、测试和发布流程见[贡献指南](CONTRIBUTING.md)。
