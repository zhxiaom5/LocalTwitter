# 部署指南

## 通用部署

在目标机器上以专用低权限用户运行 LocalTwitter。先构建前端，再构建或启动 Go 服务：

```bash
npm install --prefix web
npm run build --prefix web
go build -o localtwitter ./cmd/server
LOCALTWITTER_ADDR=:8080 \
LOCALTWITTER_CONFIG=/var/lib/localtwitter/localtwitter.config.json \
LOCALTWITTER_DB_DIR=/var/lib/localtwitter \
LOCALTWITTER_ACCESS_LOG=/var/log/localtwitter/access.log \
./localtwitter
```

创建 `/var/lib/localtwitter` 和 `/var/log/localtwitter`，仅授予运行用户所需的读写权限。不要把数据库、配置、日志或内容目录放进仓库或容器镜像；生产环境应通过系统服务、受限文件权限和备份策略管理它们。

若通过反向代理公开访问，请在部署层配置 TLS、访问控制和可信代理范围。不要直接把个人内容服务暴露到公网。

## fnOS

fnOS 打包脚本位于 `scripts/build-fnos-fpk.sh`，会输出版本化 FPK 到被忽略的 `dist/`。在确认目标设备地址和登录态均属于你后，可使用 Playwright 辅助上传：

```bash
FNOS_WEB_URL=http://fnos-host:5666 \
LOCALTWITTER_VERIFY_URL=http://fnos-host:8083 \
scripts/deploy-fnos-playwright.sh --login --headed

FNOS_WEB_URL=http://fnos-host:5666 \
LOCALTWITTER_VERIFY_URL=http://fnos-host:8083 \
scripts/deploy-fnos-playwright.sh --build --install
```

`FNOS_STORAGE_STATE` 指向的文件含浏览器会话，必须仅保存在受限的本地路径。更多脚本排查细节见 [fnos-cicd.md](fnos-cicd.md)。

## 升级与回滚

升级前备份 SQLite 数据库和配置文件。新版本先在副本数据上验证扫描和浏览流程，再替换生产二进制或 FPK。若需要回滚，停止服务，恢复上一个可用二进制/包和对应的数据库备份；不要通过 Git 回滚用户数据目录。
