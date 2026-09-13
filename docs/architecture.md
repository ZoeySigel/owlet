# Owlet 实现架构

本文描述 2026-09-06 的实际代码，替代最初的接口草案。应用同域部署：浏览器 → Caddy → Go HTTP / Worker → PostgreSQL；Worker 调用 BigModel，文件存入私有数据卷。浏览器 Canvas 排版导出。GitHub Pages 仅适用于介绍页。

## 服务与存储

Go 模块化单体，HTTP 与 Worker 同进程。连接池最多 5 条，Worker 独占一条 advisory lock 连接，以保证多个实例重叠重启时只有一个消费者。当前文本与图片都全站串行执行，满足图片并发 1，也意味着长图片任务会阻塞文字任务；面向低预算邀请演示，未做吞吐量承诺。

| 实际表 | 职责 |
|---|---|
| users / invitations / sessions | bcrypt 密码、单次邀请码、可撤销会话；令牌只存摘要 |
| documents / versions | product、brand、project JSONB 内容、revision、资料快照、完整历史 |
| assets | 随机 ID、所属用户、媒体类型与大小；文件存 DATA_DIR |
| jobs / events | 输入与费率快照、持久状态、dispatch 标记、SSE 事件 |
| quotas / ledger | 日/月桶 reserved、spent；每 job 最多一条结算流水 |
| settings / audit | 全站暂停和管理员记录 |

schema.sql 是启动执行的幂等初始迁移。后续改变已有字段须补充显式迁移，目前没有迁移版本工具。

## HTTP 接口

| 接口 | 行为 |
|---|---|
| POST /api/auth/register、/api/auth/login、/api/auth/logout | 邀请注册、登录、退出 |
| GET /api/me | 当前用户 |
| GET、POST /api/documents | 资料和项目列表、创建 |
| PUT、DELETE /api/documents/{id} | 携带 revision 保存或删除 |
| GET /api/documents/{id}/versions | 历史内容 |
| POST /api/assets；GET /api/assets/{id} | 上传、鉴权读取 |
| GET /api/usage；POST /api/quotes | 额度、单次调用预估 |
| POST、GET /api/jobs | 幂等创建、查询；管理员可用 all=true |
| POST /api/jobs/{id}/cancel | 请求取消未执行调用 |
| GET /api/jobs/{id}/events | SSE，Last-Event-ID 回放、心跳 |
| GET /api/admin；POST /api/admin/{action} | 邀请、禁用、重置、暂停、核对 |

业务读取约束 owner_id，管理员操作另查角色。写请求检查 Origin；生产 Cookie 使用 HttpOnly、Secure、SameSite=Strict。登录计算并发最多 2，来源地址每分钟最多 20 次；当前代理后的来源限制会合并为代理地址，扩大使用前应改为可信代理解析及细化限流。

## 费用一致性

人民币微元整数：1 元 = 1,000,000。北京时间用户日限 5 元、全站月限 50 元。提交事务先更新全站月桶，再更新用户日桶；只有 spent + reserved + 本次上界 ≤ cap 才成功。任一步失败整笔回滚。UNIQUE(owner_id, idem) 防止重复点击；相同键不同输入拒绝。

执行前跨日/月任务释放旧预留、在新周期重新预留，不能预留则不调用。已发出调用归属执行开始周期。结算锁住 job 行，检查 settled，释放预留并记实际支出，流水主键再次防重。

## 恢复策略

queued → running → succeeded / failed / cancelled；已发出但无法证明结果进入 outcome_unknown，保留额度。dispatched 标记在外部 HTTP 之前写入，因此崩溃窗口宁可人工核对，也不盲目重复付费。

重启时未 dispatched 的 running 重新排队；已 dispatched 的转为未知。图片 URL 先存数据库，下载失败重下同一 URL，不再次生成。管理员核对只结算。后台任务不依赖 SSE 连接存活。

真实文本使用 usage 结算，缺失 usage 保留待核对。实际费用高于预留会记录并暂停生成，因此错误费率配置仍可能破坏预算上界，启用前必须核实。模拟记录是测试记账，不能当真实支出。

## 内容与资产

项目复制商品/品牌，显式应用最新资料才改变快照。2 秒自动保存与 revision 冲突保护；历史恢复为新草稿。三个模板共用 Canvas 渲染路径预览和导出 900×1200 PNG。字体使用系统字体，尚未实现跨操作系统像素一致。

图片限制 10 MiB、20MP 并验证格式。下载仅接受 HTTPS 公网地址并检查重定向。可用磁盘低于 512 MiB 拒绝新增上传或生成。

项目最后编辑/生成后保留 30 天，前三天在界面提醒。清理和引用写入使用同一事务锁，检查资料、所有保留版本和任务结果，共享资产不会删除。JSON 引用查询偏保守，规模扩大后适合显式引用表。事件保留 30 天；费用流水保留，不因删除项目退款。
