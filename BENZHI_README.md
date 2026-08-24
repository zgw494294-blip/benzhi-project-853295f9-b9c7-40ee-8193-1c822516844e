# BENZHI_README

## 项目说明
- 项目：benzhi-project-853295f9-b9c7-40ee-8193-1c822516844e
- 项目用途：面向博物馆预防性保护团队的馆藏材料展前环境驯化服务，覆盖材料敏感性评估、分阶段温湿度适应、偏差隔离与重跑、保护复核和不可变展厅准入凭据签发。
- Go 工具链：`golang:1.23.0`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/acclimatizationd -selfcheck -addr=127.0.0.1:19127
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-853295f9-b9c7-40ee-8193-1c822516844e-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-853295f9-b9c7-40ee-8193-1c822516844e-arm64 linux/arm64
docker run -it benzhi-project-853295f9-b9c7-40ee-8193-1c822516844e-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/acclimatizationd -selfcheck -addr=127.0.0.1:19127`
