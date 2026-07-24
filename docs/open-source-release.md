# 开源发布准备记录

日期：2026-07-25

## 范围

- 基线：`7d6a07c`。
- 隔离目录：位于日常工作区同级的独立 worktree（不纳入公开文档的机器绝对路径）。
- 不包含日常工作区未提交内容，也不保留原 Git 历史。
- 许可证：MIT。

## 已识别的发布前风险

- 根目录曾跟踪 `.cache/go-build` 构建缓存。
- 部署脚本和文档包含个人局域网地址、绝对工作路径及浏览器登录态位置。
- UI 占位符包含个人目录路径。
- 原仓库没有明确的根级开源许可证。

## 验收记录

## 验收记录

- 已扫描追踪文件中的个人绝对路径、私有网段、数据库/媒体/日志/FPK 路径和高置信度密钥格式；未发现待公开的匹配项。
- 已确认 `.local/fnos/storage-state.json`、数据库、日志、`dist/`、内容目录与 `.env` 均受 `.gitignore` 保护。
- 已删除误跟踪的 `.cache/go-build` 构建缓存。
- `go test ./...`：通过。
- `npm test --prefix web`：60 个测试通过（现有 React `act(...)` 警告不影响结果）。
- `npm run build --prefix web`：通过。
- `npm run test:e2e --prefix web`：12 个 Playwright 测试通过。
- `npm audit --prefix web --omit=dev`：通过；为消除 2 个中危 React Router 漏洞，将 `react-router-dom` 升级到 7.18.1。
- 已创建无父提交的 `open-source` 根提交；`git rev-list --count HEAD` 为 1。该分支不含日常仓库的任何提交历史。
- 已完成提交树和暂存差异审查：公开树仅包含源码、测试、打包定义、配置样例与文档。
- 已创建并推送公开 GitHub 仓库：<https://github.com/zhxiaom5/LocalTwitter>。
- GitHub 验证：仓库公开、默认分支为 `open-source`、MIT 许可证已被识别；Issues 已启用，Wiki、Projects 和 Discussions 已关闭。
