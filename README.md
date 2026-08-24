# collection-acclimatization-pass

本项目为博物馆预防性保护团队提供馆藏材料展前环境驯化服务。系统将材料敏感性档案、分阶段温湿度适应、连续稳定窗口、偏差隔离与检查点重跑、保护复核和展厅准入凭据收束为一条可审计流程。数据保存在本地 WAL 风格 JSON 持久化文件中，写操作使用批次版本和 `Idempotency-Key` 防止并发重复推进。

## 构建与运行

```text
go build ./...
go run ./cmd/acclimatizationd -addr=127.0.0.1:19127 -db=./data/acclimatization.json
```

服务默认监听 `127.0.0.1:19127`，也可使用 `-addr=127.0.0.1:<port>` 或设置端口号形式的 `PORT` 环境变量。服务只接受回环监听地址。

## 自检与测试

```text
go run ./cmd/acclimatizationd -selfcheck -addr=127.0.0.1:19127
go test ./...
```

自检会启动真实回环 HTTP 服务，创建批次、登记材料、生成并冻结方案、提交稳定读数、提交复核、批准并查询凭据摘要，完成后自动退出。

批次集合 GET 支持 `status`、`venueId`、`ownerName`、`plannedStartFrom`、`plannedStartTo`、`limit` 和 `cursor` 筛选，并返回阶段进度、开放偏差和状态待办汇总。`profiles` 与 `readings` 入口同时接受单条对象或数组载荷；批量写入仍使用同一个 `If-Match-Version` 和 `Idempotency-Key`。

方案冻结前可通过 `/v1/acclimatization-batches/{batchID}/plan/diff` 获取 `planDigest` 与逐阶段差异，冻结请求体需携带当前摘要。读数历史通过批次下的 `GET /readings` 查询，支持阶段、attempt、时间、判定和分页统计。凭据查询增加 `verify=true` 及 `evidenceSection=profiles|stages|deviations|review`，用于只读复验证据完整性。
