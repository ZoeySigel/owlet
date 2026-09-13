# 真实验证记录

更新：2026-09-07。本地环境为 Windows、Node 22、便携 Go、隔离 PostgreSQL、Chrome。云端为阿里云轻量应用服务器、Ubuntu 24.04.2、PostgreSQL 16.15、Caddy 2.6.2，通过固定主机指纹的 SSH 隧道验收。

## 已通过

- Go 数据库测试 11 项：北京时间周期、真实调用缺配置拒绝、20 并发争抢最后额度、结算幂等、未知结果核对、资产隔离、跨月迁移、重启恢复分类、取消释放、共享资产清理、内网图片 URL 拦截。
- go vet 无问题；Windows 服务构建和 CGO_ENABLED=0 的 Linux amd64 交叉编译成功。
- HTTP smoke：邀请码、两用户数据隔离、提交幂等、SSE 越权拒绝、mock 任务完成与结算、版本查询、管理员权限、禁用与退出。
- TypeScript 与 Vite 生产构建通过。
- Chrome 浏览器：邀请码注册 → 商品图片上传 → 四页手动文案 → ZIP 下载 → 模拟文案 → 人工确认 → 模拟背景 → 保存与刷新 → 手机视口。局部重写与手机 PNG 下载也列入最终浏览器用例。
- 导出 ZIP 实际包含 1.png～4.png 和文案.txt；四张 PNG 均为 900×1200。
- docker compose config --quiet 通过；此检查不需要 Docker daemon，也不证明容器运行正常。

## 证据

- 正式根域名 https://owl-et.me 的 A 记录与服务器一致，Let's Encrypt 证书签发成功，公网健康接口 200。正式 HTTPS 浏览器完整用例 1 passed（49.0s），包含 Secure / HttpOnly / SameSite=Strict Cookie 检查，未忽略证书错误。

- 云端 11 项 Go 测试全部通过；HTTP smoke 全部通过；真实服务器浏览器用例 1 passed（36.0s，包括局部重写与手机 PNG 下载）。
- 提交 mock 任务后实际执行 systemctl restart owlet：任务成功，重复提交复用原 job，账本仅一条；见 scripts/server-restart-smoke.mjs。
- 备份时间 20260907T133724Z：数据库 dump 与资产包校验通过，在独立库恢复；六类业务表记录数一致，两张资产 SHA256 一致；原浏览器用户可登录恢复服务并读取旧项目和全部引用图片。演练服务、恢复库及目录随后清理，正式库未覆盖。
- 首份备份经 SSH 复制至本机并 AES-256-GCM 加密，解密后的 SHA256 与原包一致。密钥与备份在 .tools 下，不进入版本控制；异机备份目前只做了这一次，不宣称已自动同步。

- [桌面编辑](demo/editor-desktop.png)、[工作台](demo/workspace-desktop.png)、[手机查看](demo/editor-mobile.png)
- [浏览器操作实录](demo/video.webm)、[手动导出 ZIP](demo/four-pages.zip)
- 可复现用例：backend/jobs_test.go、scripts/api-smoke.mjs、web/e2e/creation.spec.ts。

示例商品瓶身由测试代码绘制，标注 OWLET DEMO，不是真实商品照片。模拟模型输出固定示例文案与渐变背景，未产生真实 API 费用。mock 数字仅为预算状态机的模拟记账。

## 尚未验证

- BigModel 账号权限、实际模型响应/usage、图片尺寸支持、生成质量、价格上界、真实账单核对。
- 长时间稳定性、整台主机重启、持续负载 CPU/RSS 与容量上限。已完成的是应用服务重启，不是主机重启。
- Docker 镜像构建运行仍未验证；实际服务器走原生 systemd 部署。
- GitHub Actions 云端运行、go test -race（本地没有 C 编译环境，CI 已配置）、iOS Safari 的下载行为。
- 上游 HTTP 故障只通过状态/账本用例验证恢复决策，未对真实付费服务注入故障。

## 已知取舍

当前 Worker 全局串行；文案与图片共享排队。素材引用使用 JSON 检查，保守保留可能多于必要文件。系统字体可能在不同设备有差异。登录限流在同一代理后共享计数。大文件、长文案与多人真实使用仍需上线前人工试用，不能从功能测试推导出“高并发生产可用”。
