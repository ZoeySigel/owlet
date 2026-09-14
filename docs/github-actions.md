# GitHub Actions

仓库： https://github.com/ZoeySigel/owlet

## 自动测试

推送 main、提交 PR 或手动运行 Validate Owlet 时执行：

1. Ubuntu + PostgreSQL 16 下的 Go race 测试与 go vet。
2. 发布包路径穿越、符号链接和版本不匹配防护测试。
3. React/TypeScript 构建、Linux amd64 Go 二进制构建。
4. 运行隔离 mock 服务，执行 HTTP 隔离测试和 Chromium 四页创作端到端测试。
5. 仅在全部通过后保存 owlet-提交SHA 构建产物，保留 14 天；失败诊断保留 3 天。

CI 使用临时测试凭证及模拟模型，不接触生产数据库，也不产生模型费用。

## 手动部署

打开 Actions → Deploy Owlet (manual) → Run workflow，选择 main。工作流先复用完整测试流程，然后部署该次运行对应的同一个提交与产物。普通 push 不会部署；其他分支的部署请求跳过。部署串行执行，正在进行的部署不会被新点击中断。

部署任务绑定名为 OWLET 的 Environment。三个密钥可在 Settings → Environments → OWLET → Environment secrets 中设置，也兼容 Repository Secrets。OWLET_DEPLOY_KEY 是本机 .tools/ssh/owlet_actions_ed25519 文件全文（包含 BEGIN/END 行），禁止提交到代码或通过聊天发送。

另外两个 Secret：OWLET_DEPLOY_HOST 为服务器 IP，OWLET_KNOWN_HOST 为已核实的 known_hosts 记录（主机名、公钥类型、公钥三部分）。全部使用 Secrets，以遮盖公开 Actions 日志中的值。更换服务器必须通过可信渠道重新核对公钥，不自动信任 ssh-keyscan 结果。

## 服务器权限与更新

专用 owlet-ci 账号、公钥强制命令与 sudoers 只允许调用 root 所有的部署入口。SSH 无交互 Shell、无端口转发，发布包经大小、文件类型、路径及提交 SHA 校验。仅允许 owlet 二进制、web/ 与 REVISION，不接收环境配置、数据库或部署脚本更新。

每次更新先执行一致备份，然后在 /opt/owlet/releases/ 下保存新版本，以 current 符号链接切换。服务仍以 owlet 用户运行；健康检查失败回退应用文件并重启。数据库不会自动回滚：涉及不兼容迁移时需要单独制定迁移与恢复步骤。历史版本目前保留，后续应按磁盘使用情况清理已确认不用的版本。

应用代码本身可访问 Owlet 用户的数据和运行时模型凭证，因此只有可信维护者能修改 main 并发起部署。Actions 使用的部署密钥与日常管理密钥不同，可独立撤销。

## 运维入口

- 构建、浏览器和部署失败先看对应 Actions 步骤日志。
- 公网版本：GET https://owl-et.me/revision.txt （首次采用版本化部署后可用）。
- 服务日志：sudo journalctl --namespace=owlet -u owlet -n 100。
- 原 /etc/owlet/app.env、/var/lib/owlet/assets 与 PostgreSQL 数据保持在原位置。
- 修改受限部署脚本需由管理员审查后在服务器安装，普通 Actions 发布不能更新 root 部署脚本。
