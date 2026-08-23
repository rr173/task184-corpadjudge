# task184-corpadjudge — 语言语料标注准则分歧裁决台

面向语料库语言学家的多人标注分歧裁决与一致性基准管理平台。负责人发布准则版本与条款；
标注员对语料片段提交词性/指代/语义角色标签；系统按片段归并一致项与冲突项，展示各标签
引用的准则条款；裁决者引用条款作出决定、将案例规则化并冻结为基准集；准则更新后自动标出
需要重审的案例，而旧基准集始终保留原决定。

## 业务闭环

1. 负责人创建并发布准则版本，附加条款（草拟 → 已发布 → 存在歧义 → 已废止）。
2. 语料片段入库（指纹幂等，重复提交返回既有片段）。
3. 标注员提交标签（同一标注员对同一版本重复投票幂等；互斥标签层被拒绝）。
4. 系统按片段归并：单标签 → 一致；多标签 → 创建分歧矩阵。
5. 裁决者引用条款决定标签，案例规则化并冻结入基准快照，片段随之冻结。
6. 准则更新 → 影响分析标出引用改写条款的案例 → 生成重审任务；旧基准保留原决定。

## 标准命令

```bash
export GOTOOLCHAIN=local
export CGO_ENABLED=0

go build ./...
go vet   ./...
go test  ./...
go run ./cmd/corpadjudge --smoke-test          # 端到端冒烟 + 重启恢复验证
go run ./cmd/corpadjudge --addr :8080 --db ./data/corpadjudge.db
```

## API 一览（前缀 /api）

| 模块 | 入口 |
| --- | --- |
| 准则 | POST/GET /api/guidelines，POST /api/guidelines/{id}/publish\|ambiguous\|revoke，POST/GET /api/guidelines/{id}/clauses，PATCH /api/clauses/{id}，POST /api/guidelines/{id}/impact |
| 片段 | POST/GET /api/spans，GET /api/spans/{id}，POST /api/spans/{id}/annotations |
| 标注 | PATCH /api/annotations/{id}，POST /api/annotations/{id}/submit，GET /api/annotations |
| 分歧 | GET /api/disputes，GET /api/disputes/{id}，POST /api/disputes/{id}/matrix\|refs |
| 裁决 | POST/GET /api/cases，GET /api/cases/{id}，POST /api/cases/{id}/decide\|formalize |
| 基准 | POST/GET /api/baselines，GET /api/baselines/{id}，POST /api/baselines/{id}/freeze，GET /api/baselines/{id}/export |
| 重审 | GET /api/rereviews，POST /api/rereviews/{id}/resolve |
| 自检 | GET /api/stats，GET /api/health，POST /api/selfcheck |

## 持久化

SQLite（`modernc.org/sqlite`，CGO 无关）11 张表：准则版本/条款、语料片段、标注、
分歧/矩阵、裁决案例、基准快照/案例、重审任务。重复标注按 vote_key 幂等，片段按指纹
去重；冻结案例绑定原片段指纹与原准则版本，准则更新不影响旧基准。

## 目录结构

```
env/
├── cmd/corpadjudge/main.go
└── internal/
    ├── model/      实体与统一错误
    ├── store/      SQLite 持久化（迁移 + CRUD）
    ├── guideline/  准则版本生命周期 + 影响分析
    ├── corpus/     片段管理 + 指纹 + 归并
    ├── annotation/ 标注提交 + 幂等 + 层级混淆防护
    ├── dispute/    分歧查询 + 矩阵计算
    ├── adjudication/ 裁决 + 并发冲突检测
    ├── baseline/   基准冻结 + 重审任务
    ├── service/    业务闭环编排
    └── httpapi/    REST 路由（/api）
```
