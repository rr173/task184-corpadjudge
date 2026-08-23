# task184-corpadjudge — 评测说明（BENZHI）

语言语料标注准则分歧裁决台：负责人发布准则版本，标注员提交标签，系统归并分歧矩阵，
裁决者引用条款作出决定并冻结一致性基准；准则更新自动标出重审案例，旧基准保留原决定。

## 标准命令

```bash
export GOTOOLCHAIN=local CGO_ENABLED=0
go build ./...
go vet   ./...
go test  ./...
go run ./cmd/corpadjudge --smoke-test
```

## 运行服务

```bash
go run ./cmd/corpadjudge --addr :8080 --db ./data/corpadjudge.db
curl http://localhost:8080/api/health
curl http://localhost:8080/api/stats
curl -X POST http://localhost:8080/api/selfcheck
```

## --smoke-test 契约

不启动长驻服务，执行真实业务闭环并验证持久化与重启恢复：

1. 创建并发布准则版本 + 条款；
2. 创建语料片段（指纹幂等验证：重复创建返回同一 ID）；
3. 三位标注员对同一片段提交两种标签 → 归并出分歧（矩阵 2 个标签）；
4. 重复投票幂等验证：同一标注员同一标签只产生一条记录；
5. 裁决 → 规则化 → 基准快照冻结（片段随之冻结）；
6. 发布新准则版本 → 影响分析标出 1 个需重审案例；
7. **关闭数据库并重新打开**，验证准则/片段/案例/基准全部恢复；
8. 校验：旧基准案例仍绑定原准则版本与片段指纹；冻结案例触发冲突守卫；
9. 创建并解决重审任务，重审队列清空。

全部通过后以退出码 0 结束并打印 `SMOKE TEST PASSED`；任一失败打印原因并退出码 1。

## Docker 双架构

```bash
bash build_benzhi_docker.sh my-project linux/amd64
docker run --rm my-project:latest --smoke-test
```

Dockerfile 采用单阶段构建（golang:1.26.3-bookworm），ENTRYPOINT 为二进制，
CMD 为 `--smoke-test`；构建镜像后仅需 `docker run --rm <image> --smoke-test`
即可完成双架构（linux/amd64、linux/arm64）验证。
