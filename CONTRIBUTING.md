# Contributing to LocalTwitter

感谢贡献。提交前请先阅读 [AGENTS.md](AGENTS.md) 中的项目边界和测试要求。

## 开发流程

1. 从最新默认分支创建主题分支。
2. 为功能或缺陷先设计测试用例，再实现代码。
3. 不提交数据库、媒体、Cookie、Token、日志、构建物、个人绝对路径或本地部署配置。
4. 运行并记录：`go test ./...`、`npm test --prefix web`、`npm run build --prefix web` 以及相关 Playwright E2E。
5. 提交清晰、范围单一的变更；在 Pull Request 中说明用户可见行为、测试证据与风险。

## 代码约定

- Go 代码运行 `gofmt`；前端遵循现有 TypeScript 和测试模式。
- 不新增缓存、消息队列或额外服务来解决局部问题。
- 服务端强制输入校验和权限边界；前端不假设未定义的服务端行为。
- 新增配置项必须有安全默认值和文档，且真实凭据只能保存在被忽略的本地环境中。

安全问题请按照 [SECURITY.md](SECURITY.md) 私下报告，不要作为普通 PR 或公开 Issue 提交。
