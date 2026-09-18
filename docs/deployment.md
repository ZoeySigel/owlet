# 部署与恢复

2026-09-07 已在阿里云轻量应用服务器采用原生 systemd 方案部署，部署代码位于 deploy/native；具体服务器身份和运维实录不公开。本文 Docker 部分保留为备选方案，容器尚未运行验证。BigModel 启用条件仍适用。

## 2 核 2 GB 方案

Caddy 同时托管 React 静态文件并反向代理 /api 到 Go，PostgreSQL 不暴露公网端口。Compose 限制 DB 512 MiB、Go 384 MiB、Caddy 128 MiB；其余留给系统。这些是配置值，不是实测资源数据。Node 和 Go 构建应在开发机或 CI，服务器仅运行成品镜像。

先核实服务器 IP、SSH 用户/端口/密钥、现有 80/443 占用、可用磁盘和系统负载。域名 owl-et.me 的 A/AAAA 应与可达服务器一致。安全组开放 80/443，SSH 仅保留实际需要的来源。配置之前检查已有站点，不覆盖其他服务。

## 制作并启动镜像

在有 Docker 的开发机执行：

```sh
docker build -t owlet-app:local backend
docker build -t owlet-web:local web
docker save -o owlet-images.tar owlet-app:local owlet-web:local
```

将镜像包、compose.yaml、scripts/backup.sh、.env.example 传至服务器独立目录。服务器加载镜像，复制 .env.example 为 .env，并设置随机密码、SITE_DOMAIN=owl-et.me，保留 MODEL_MODE=mock。chmod 600 .env。数据库密码建议随机十六进制，避免 URL 特殊字符。

```sh
docker load -i owlet-images.tar
docker compose config --quiet
docker compose up -d --no-build
docker compose ps
curl --fail https://owl-et.me/api/health
docker stats --no-stream
df -h
```

Caddy 在域名解析、80/443 可达时自动申请 HTTPS。生产不要叠加 compose.dev.yaml。Docker 自启和 restart: unless-stopped 提供重启恢复；日志限制每服务 10MB × 3。应用还在磁盘不足 512MiB 时拒绝新增文件/任务，仍需部署后用外部监控提醒磁盘、水位和健康状态。

## 启用 BigModel

文本模型通过 BIGMODEL_TEXT_MODEL 指定。图片模型和尺寸通过 BIGMODEL_IMAGE_MODEL、BIGMODEL_IMAGE_SIZE 配置，默认 glm-image、1152x1536（3:4）。调用前校验模型尺寸约束；排队后修改模型或尺寸会拒绝旧任务并释放预留。参数尚待账号实测，不能将协议测试视为实际权限验证。

先核实账号实际费率、文字最大上下文、输出上限与图片权限。配置 BIGMODEL_API_KEY、BIGMODEL_TEXT_MODEL、TEXT_CONTEXT_TOKENS、TEXT_MAX_TOKENS，以及输入/输出每百万 token 的微元价格；TEXT_RESERVE_MICRO 必须覆盖最大上下文输入与输出费用。IMAGE_PRICE_MICRO 是一张图的已核实费用，不应沿用示例值当实际价格。

流式文本必须同时取得结束标记、finish_reason 与完整非负 usage；连接中断、异常事件或缺少用量保留预留并进入待核对。已完成但四页结构不合格或输出被截断的文案计入已发生费用并标记失败，不自动重发。上游 HTTP 错误同样保守等待核对。图片下载失败只重试已有 URL，不重新生成。

密钥仅保存在服务器 /etc/owlet/app.env（root 所有、0600），不需要加入 GitHub Actions。保存密钥不等于启用真实调用；核实配置与正式账本隔离后再切换模式。参考 [对话接口](https://docs.bigmodel.cn/api-reference/模型-api/对话补全)、[图片接口](https://docs.bigmodel.cn/api-reference/模型-api/图像生成) 与 [官方价格](https://bigmodel.cn/pricing)，实际账号费率仍需核对。

金额换算：1 元 = 1,000,000 微元；每百万 token 收 X 元，则费率变量 = X × 1,000,000。代码目前以完整最大上下文估算，较保守；若单次上界超过个人 5 元会拒绝，需要选择合适模型。

全部核实后设置 MODEL_MODE=bigmodel、PAID_CALLS_VERIFIED=true 并重启 app。模式或费率改变后，旧排队任务会失败并释放预留。不要在有未核对任务时切换环境。正式演示使用干净的数据库和独立数据卷，保留原模拟库用于测试，避免混合 mock 账本和真实账单。

第一次仅生成一篇文案与一张背景，手动对账后再开放邀请码。usage 缺失、超时或上游结果不明时，到后台核对；禁止通过重复生成猜测是否成功。密钥不得进入前端、截图、视频、Git 或文档。

## 备份

从部署目录执行 `sh scripts/backup.sh`。脚本暂时停止 app，使数据库与文件组成一致快照，然后输出数据库 dump 与资产 tar.gz，退出时恢复 app。尚未在服务器执行过。建议每日低峰运行，最多保留 7 天，加密复制到独立机器，监控命令退出码；不要只在原服务器保留唯一副本。

## 独立恢复演练

不要对正在使用的数据卷执行恢复。建立单独目录与 Compose project（例如 owlet-restore-check），配置不同 SITE_DOMAIN 或不启动 web；只启动 db，再导入选定的一对备份：

```sh
docker compose -p owlet-restore-check up -d db
docker compose -p owlet-restore-check exec -T db pg_restore -U owlet -d owlet --no-owner --exit-on-error < backups/SELECTED.dump
docker compose -p owlet-restore-check run --rm --no-deps --entrypoint tar app -C /data -xzf - < backups/SELECTED.assets.tgz
```

SELECTED 必须替换为同一时间戳。恢复库先只读检查用户、项目、版本、任务、额度与资产文件是否匹配。首次启动恢复 app 前，把 MODEL_MODE=mock、PAID_CALLS_VERIFIED=false 且清空 API key，防止演练发起付费请求。不能直接用恢复测试数据覆盖正式库。

在隔离端口完成登录、旧版本恢复、图片读取、导出检查；记录备份时间、文件哈希、恢复耗时和结果。云端恢复演练完成后，才可把“备份恢复已验证”写入简历。
