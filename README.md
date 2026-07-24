# LocalTwitter

LocalTwitter 是一个由单个 Go 服务承载的局域网本地内容浏览器：它索引使用者自己配置的本地目录，在浏览器中展示内容、作者、媒体和更新状态。前端资源在构建后嵌入同一服务，不依赖外部数据库或缓存。

> 本项目不附带 Twitter/X 登录信息、下载内容、数据库、Cookie 或媒体文件。使用者须自行确认其数据来源、账户使用方式及部署行为符合适用法律和相关平台条款。

## 快速开始

前置条件：Go（版本见 `go.mod`）和 Node.js（建议使用当前 LTS）。

```bash
git clone <your-fork-or-repository-url> LocalTwitter
cd LocalTwitter
cp .env.example .env # 仅作为本地工具变量参考；不要提交 .env
npm install --prefix web
npm run build --prefix web
go run ./cmd/server
```

打开 `http://localhost:8080`。首次使用时在设置页选择运行该服务机器上的数据库目录和本地内容目录；数据库会在所选目录创建。默认本地配置文件为 `localtwitter.config.json`，已被 Git 忽略。

完整使用说明见 [docs/user-guide.md](docs/user-guide.md)，生产与 fnOS 部署见 [docs/deployment.md](docs/deployment.md)。

## 开发与验证

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
npm run test:e2e --prefix web
```

前端调试服务器：

```bash
npm run dev --prefix web
```

## 配置

服务通过环境变量配置；可复制 `.env.example` 作为本地命令参考。不要将 `.env`、`localtwitter.config.json`、SQLite 数据库、浏览器 `storageState`、Cookie、Token 或代理凭据提交到仓库。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `LOCALTWITTER_ADDR` | `:8080` | HTTP 监听地址 |
| `LOCALTWITTER_CONFIG` | `localtwitter.config.json` | 本地配置文件路径 |
| `LOCALTWITTER_DB_DIR` | `.` | SQLite 数据库存放目录 |
| `LOCALTWITTER_WEB_DIR` | 未设置 | 可选的外部前端资源目录 |
| `LOCALTWITTER_ACCESS_LOG` | `access.log` | 可选访问日志路径 |
| `LOCALTWITTER_DOWNLOADER` | 未设置 | 可选下载器入口路径 |

## 目录结构

```text
cmd/server/                  Go 服务入口
internal/server/             HTTP、应用逻辑、SQLite 与扫描能力
web/src/                     React 页面、组件、接口与测试
web/e2e/                     Playwright 端到端测试
scripts/                     构建、验证和可选 fnOS 自动化
packaging/fnos/              fnOS 打包定义
docs/                        用户、部署、测试与安全文档
```

构建输出、数据库、下载内容、日志和本地浏览器登录态均被 `.gitignore` 排除。

## 贡献与安全

- 贡献流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。
- 漏洞报告请遵循 [SECURITY.md](SECURITY.md)，不要在公开 Issue 中发布敏感信息。
- 本项目按 [MIT License](LICENSE) 发布。
