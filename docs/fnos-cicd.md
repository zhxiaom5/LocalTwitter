# LocalTwitter fnOS Playwright 自动部署

本项目的轻量 CI/CD 采用本机 Playwright 登录态方案：

1. 首次用可见浏览器登录 fnOS，并保存 `storageState`。
2. 后续本机或自动化任务用 headless Playwright 上传 FPK 并确认安装/升级。
3. FPK 仍由 `scripts/build-fnos-fpk.sh` 生成，保持自动 patch 版本提升。
4. 未传 `--fpk` 时，部署脚本会自动选择 `dist/` 下版本号最大的 `localtwitter-v*-x86_64.fpk` 或 `localtwitter-v*-root-x86_64.fpk`，不会使用 latest alias。

## 首次保存登录态

```bash
FNOS_WEB_URL=http://fnos-host:5666 \
scripts/deploy-fnos-playwright.sh --login --headed
```

浏览器打开后登录 fnOS。登录完成后回到终端按回车，脚本会保存：

```text
.local/fnos/storage-state.json
```

该文件包含本机登录态，已被 `.gitignore` 排除，不能提交。

## 打包并无头安装

```bash
FNOS_WEB_URL=http://fnos-host:5666 \
LOCALTWITTER_VERIFY_URL=http://fnos-host:8083 \
scripts/deploy-fnos-playwright.sh --build --install
```

安装流程会按 fnOS 应用中心的实际路径执行：

1. 打开 fnOS 桌面并进入“应用中心”。
2. 点击左侧底部“手动安装”。
3. 在弹窗中选择“从电脑上传”，上传 FPK。
4. 出现“未经验证应用的安全提示”时点击“同意”。
5. 出现“更新 LocalTwitter - 检查设置”时保留“更新完成后立即启用”，点击“确定”。
6. 等待 fnOS 自动更新和启用；脚本读取 FPK 内 `manifest` 的版本号，并与应用中心详情页的“当前版本”比较，一致即视为升级成功。

如果已经打包好，可以只安装：

```bash
scripts/deploy-fnos-playwright.sh --install
```

也可以显式指定某个版本化 FPK：

```bash
scripts/deploy-fnos-playwright.sh --install --fpk dist/localtwitter-v0.1.12-x86_64.fpk
```

## Dry-run

Dry-run 只检查 FPK、fnOS 页面可访问性和登录态，不点击安装：

```bash
scripts/deploy-fnos-playwright.sh --install --dry-run
```

Dry-run 会确认能够进入应用中心、看到“手动安装”入口，并能读取待安装 FPK 的 manifest 版本，但不会点击“手动安装”，不会上传 FPK。

## Selector 自测

脚本内置一个本地 mock fnOS 页面，用于验证中文弹窗选择器：

```bash
scripts/deploy-fnos-playwright.sh --self-test-selectors
```

## 调试

安装流程默认 headless。需要看浏览器时加 `--headed`：

```bash
scripts/deploy-fnos-playwright.sh --install --headed
```

失败时会保存截图到：

```text
output/fnos-deploy/
```

如果 fnOS 页面文案变化，可以用环境变量覆盖按钮匹配文本：

```bash
FNOS_APP_CENTER_TEXT=应用中心 \
FNOS_MANUAL_INSTALL_TEXT=手动安装 \
FNOS_UPLOAD_FROM_COMPUTER_TEXT=从电脑上传 \
FNOS_SECURITY_AGREE_TEXT=同意 \
FNOS_UPDATE_CONFIRM_TEXT=确定 \
scripts/deploy-fnos-playwright.sh --install --headed
```

## 轻量自动化建议

不使用 GitHub runner 时，推荐把本机命令作为 Codex 自动化或本机定时任务的执行内容：

```bash
cd /path/to/LocalTwitter
scripts/deploy-fnos-playwright.sh --build --install
```

如果登录态过期，脚本会提示重新运行 `--login --headed`。

## 故障排查

- `Cannot read properties of undefined (reading 'launch')`：说明脚本没有正确加载 Playwright runtime。请确认使用的是修复后的 `scripts/fnos-browser-install.mjs`，并运行 `npm install --prefix web` 后重试。
- 卡在“未经验证应用的安全提示”：脚本应该只在弹窗内点击“同意”，不会回退到背景窗口。失败时查看 `output/fnos-deploy/` 下的 `security-warning-failed` 或 `install-failed` 截图。
- 安装实际成功但脚本失败：现在成功条件以应用中心 LocalTwitter 详情页的“当前版本”和 FPK manifest 版本一致为准；`LOCALTWITTER_VERIFY_URL` 只做附加健康检查，失败会打印 warning，不会覆盖版本一致的安装结论。
- 默认安装包不符合预期：确认 `dist/` 下存在 `localtwitter-v<version>-x86_64.fpk` 或 `localtwitter-v<version>-root-x86_64.fpk`。脚本会忽略 `localtwitter-x86_64.fpk` 和 `localtwitter-root-x86_64.fpk`。
