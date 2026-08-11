# OfferPilot 离线 Eval Harness

`backend/evals` 是独立于生产面试服务的确定性质量门禁。它读取固定、脱敏、合成的
面试输出，不加载模型配置、不调用 provider，也不修改面试会话或 SQLite 数据。

v0.3.0-alpha.1 默认语料包含 30 个案例和 90 道题，完整覆盖：

- 模式：`knowledge`、`projects`、`mixed`
- 职级：`junior`、`mid`、`senior`
- 组合：全部 9 个 mode × seniority 组合

## 运行

在 `backend` 目录执行：

```bash
go run ./cmd/offerpilot-eval
go run ./cmd/offerpilot-eval -pretty=false
go run ./cmd/offerpilot-eval -corpus ./evals/corpus/v0.3.0-alpha.1.json
go run ./cmd/offerpilot-eval -schema
```

命令只向标准输出写 JSON report。退出码含义：

| 退出码 | 含义 |
|---|---|
| `0` | corpus 合法且全部阈值通过 |
| `1` | corpus 合法，但至少一个质量阈值失败 |
| `2` | 参数、文件、JSON、schema 版本或结构校验失败 |

默认阈值来自 corpus。排查或评审外部 corpus 时可使用
`-max-duplicate-rate`、`-min-evidence-validity`、
`-min-mode-coverage`、`-min-seniority-coverage`、
`-min-matrix-coverage`、`-min-mode-conformance` 和
`-max-privacy-hits` 覆盖阈值。发布门禁不得通过降低阈值来放行。

## 指标

| 指标 | 定义 |
|---|---|
| `duplicateQuestionRate` | 整个 corpus 内，经大小写、标点和空白归一化后的重复题数 / 总题数 |
| `evidenceValidityRate` | 引用存在、没有重复引用且 evidence kind 支持题型的问题数 / 总问题数 |
| `modeCoverageRate` | corpus 实际覆盖的必需模式比例 |
| `seniorityCoverageRate` | corpus 实际覆盖的必需职级比例 |
| `modeSeniorityCoverageRate` | corpus 实际覆盖的必需模式 × 职级组合比例 |
| `modeConformanceRate` | 题型组成符合案例模式的案例比例 |
| `privacyMarkerHitCount` | 问题和公开文本命中的标准或案例私有 marker 总数 |

隐私 finding 只输出 marker 的短 SHA-256 fingerprint，不回显私有 marker 原文。
默认语料还通过测试保证 90 道题跨案例归一化后互不重复。

## Corpus 约束

结构定义在 `backend/evals/schema/corpus.schema.json`，当前 schema 版本为
`1.0.0`。解码器拒绝未知字段、多个 JSON 根值、重复 ID、非法枚举、无证据目录、
空问题和不受支持的 schema 版本。默认 corpus 被 `go:embed` 固化进 CLI，运行结果
不依赖工作目录、网络或 provider 状态。

修改 corpus 时必须：

1. 使用合成或可靠脱敏材料，不能提交真实 JD、简历、回答或参考答案。
2. 升级 `corpusVersion`，保留旧版本文件以便比较发布结果。
3. 运行 `go test ./evals ./cmd/offerpilot-eval -count=1`。
4. 运行默认 CLI，并把 JSON 指标归档到候选版本验证记录。

## 能力边界

该门禁能稳定发现重复题、悬空或错型 evidence、覆盖缺口和已知隐私 marker 泄漏，
但不评价问题的语义相关性、深挖强度、可回答性、事实正确性或真实模型延迟。上述
质量仍需冻结真实 provider/model/prompt 后的重复运行、人工双盲评分和检索标注集。
离线 gate 通过只代表结构基线合格，不代表 v0.3.0 GA 的全部发布条件已满足。
