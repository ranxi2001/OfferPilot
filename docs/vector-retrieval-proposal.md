# OfferPilot 知识检索与向量数据库提案

日期：2026-06-18

## 结论

不采用本地 hash embedding 作为正式方案。

原因很直接：hash 向量只能做非常粗糙的词面近似，不能稳定召回语义相近但表达不同的内容。例如“Agent 上下文压缩”和“长上下文记忆治理”在语义上相关，但 hash 特征很容易召回失败。它最多适合作为单元测试里的离线替身，不适合作为 OfferPilot 的真实知识检索能力。

正式方案建议采用：

- 关键词检索：SQLite FTS5
- 向量检索：SQLite + sqlite-vec
- embedding 模型：优先使用流行的真实 embedding 模型，例如 BGE-M3、Jina Embeddings v3、Qwen3 Embedding
- 检索策略：FTS5 + 向量召回混合排序，后续再加 rerank

这样可以保留 SQLite 的轻量部署优势，同时具备真实语义召回能力。

## 为什么不是 hash

hash embedding 的问题：

- 不理解语义，只把 token 映射到固定维度。
- 中文同义改写、英文技术术语变体、面试表达口语化时召回不稳定。
- 维度冲突不可解释，召回质量很难调参优化。
- 评估时可能看起来“能跑”，但真实问题一变就会漏召回。

因此仓库里不应提交 hash 生成的 `data/agent.db`，也不应把 hash provider 作为默认 embedding provider。

## 推荐技术路线

### 第一阶段：SQLite FTS5 + sqlite-vec

适合当前 OfferPilot：

- 本地文件型数据库，便于随项目分发和调试。
- 不需要额外部署 Qdrant、Milvus、Weaviate 等服务。
- 知识库规模目前不大，SQLite 足够。
- `sqlite-vec` 可以提供真实向量近邻搜索，不再用 JS 内存 cosine 扫描。

数据建议落在：

```text
data/agent.db
```

数据库内容建议包含：

- `knowledge`：Markdown 解析后的 chunk。
- `knowledge_fts`：FTS5 关键词索引。
- `embeddings`：chunk 与 embedding 模型、维度、更新时间的元数据。
- `embedding_vec`：sqlite-vec 向量索引。

### 第二阶段：混合召回

查询流程：

1. 用 FTS5 召回关键词强相关内容。
2. 用真实 embedding 向量召回语义相关内容。
3. 合并去重。
4. 按加权分数排序。
5. 返回 top-k 给 Agent。

建议初始权重：

```text
final_score = 0.55 * vector_score + 0.45 * fts_score
```

后续可以根据评测集调整。

### 第三阶段：可选 rerank

当知识库扩大后，引入 rerank：

- BGE reranker
- Jina reranker
- Qwen reranker

流程变成：

```text
FTS5 top 30 + Vector top 30 -> 合并 top 50 -> rerank -> top 5
```

## embedding 模型选择

### 推荐 1：BGE-M3

适合中文、英文、代码和长文本混合检索，是目前很常见的开源 embedding 选择。

可用方式：

- 本地 Ollama / vLLM / Xinference 部署。
- 使用国内云厂商的 OpenAI-compatible embedding 服务。

优点：

- 中文效果好。
- 开源生态成熟。
- 适合 RAG 知识库。

注意：

- 本地部署需要机器资源。
- 需要确认最终服务接口是否兼容 OpenAI `/embeddings`。

### 推荐 2：Jina Embeddings v3

适合快速接入真实 embedding 服务。

优点：

- API 形态接近 OpenAI。
- 多语言表现较好。
- 接入成本低。

注意：

- 需要 Jina API Key。
- 需要把 `EMBEDDING_BASE_URL` 与 `EMBEDDING_MODEL` 独立于聊天模型配置。

### 推荐 3：Qwen3 Embedding

适合国内中文场景和云服务生态。

优点：

- 中文效果强。
- 国内平台可用性通常更好。
- 后续如果聊天模型也走国内兼容 OpenAI 服务，配置风格一致。

注意：

- 需要确认具体平台提供的模型名、维度、价格和 OpenAI-compatible 兼容程度。

## 不建议现在上独立向量数据库

Qdrant、Milvus、Weaviate 都是成熟方案，但当前 OfferPilot 不建议第一步就上：

- 增加部署复杂度。
- 本地开发需要额外服务。
- 当前知识库规模还没有到必须拆出向量数据库的程度。
- SQLite 更方便随仓库调试、打包、迁移。

等知识库达到数十万 chunk、需要多用户隔离、需要在线更新和高并发召回时，再迁移到 Qdrant 更合适。

## 配置建议

新增独立 embedding 配置，不复用聊天模型配置：

```env
EMBEDDING_PROVIDER=openai-compatible
EMBEDDING_BASE_URL=https://api.example.com/v1
EMBEDDING_API_KEY=sk-...
EMBEDDING_MODEL=bge-m3
EMBEDDING_DIMENSIONS=1024
```

如果使用本地 Ollama：

```env
EMBEDDING_PROVIDER=ollama
EMBEDDING_BASE_URL=http://localhost:11434
EMBEDDING_MODEL=bge-m3
EMBEDDING_DIMENSIONS=1024
```

聊天模型继续使用：

```env
OPENAI_BASE_URL=https://api.ai.tosky.top/v1
OPENAI_MODEL=gpt-5.5
```

## 明天实施计划

1. 移除正式代码里的 hash embedding provider。
2. 保留或接入 `sqlite-vec`，但只接受真实 embedding 写入。
3. 增加 OpenAI-compatible embedding provider。
4. 增加 Ollama embedding provider，方便本地跑 BGE-M3。
5. 调整 `embed` CLI：
   - 未配置真实 provider 时直接失败。
   - 提示用户配置 `EMBEDDING_*`。
   - 不再静默 fallback 到 hash。
6. 增加 `--rebuild` 参数，用于重建 embedding 表和 sqlite-vec 表。
7. 删除 hash 生成的 `data/agent.db`，重新用真实 embedding 构建。
8. 增加一个小型召回评测集，至少覆盖：
   - Agent Loop
   - RAG
   - 上下文管理
   - 工具调用
   - 子 Agent
   - ASR/TTS 面试诊断

## 验收标准

真实向量库完成后，至少满足：

- `npm run build-kb` 可以生成知识库 chunk。
- `npm run embed` 只使用真实 embedding provider。
- `data/agent.db` 中有 FTS5 与 sqlite-vec 数据。
- 查询“长上下文如何压缩”能召回上下文管理相关文档。
- 查询“工具调用失败怎么恢复”能召回错误处理、重试和工具治理相关文档。
- 查询“录音诊断流程”能召回 ASR/TTS 和 Web 录音诊断内容。
- 代码里没有 hash embedding 作为默认路径。

## 当前阻塞项

当前 `ai.tosky.top` 主要用于聊天模型 `gpt-5.5`，没有确认可用的 embedding 模型。

需要确认其中一个：

- 一个可用的 OpenAI-compatible embedding Base URL、API Key、模型名和维度。
- 或本机安装 Ollama 并拉取 `bge-m3`。
- 或选择 Jina / Qwen / SiliconFlow 等平台提供的 embedding 模型。

确认后再生成真实 `data/agent.db`，不要提交 hash 版本数据库。

---

## 向量检索发展规划

### 当前状态（Phase 0）

| 项目 | 状态 |
|:---|:---|
| 知识库 | 411 条 × 7 维度，已入库 SQLite |
| 关键词检索 | SQLite FTS5，已实现 |
| 向量检索 | OpenAI text-embedding-3-small + JS 内存 cosine，已实现 |
| 检索性能 | 411 条全量扫描 < 1ms，无瓶颈 |

当前方案完全满足需求。向量存储在 SQLite `embeddings` 列（序列化 Float32Array），检索时反序列化后 JS 逐条计算 cosine similarity。

---

### Phase 1：sqlite-vec 原生向量索引

**触发条件**：知识库增长到 5,000+ 条，或需要 DiskANN/HNSW 级别检索速度

**改动**：
- 引入 `sqlite-vec` 扩展（C 编译，通过 better-sqlite3 加载 `.so`/.`dylib`）
- 新建 `vec_knowledge` 虚拟表，使用 IVF 或 HNSW 索引
- `searchAsync()` 从 JS cosine 切换为 SQL `vec_distance_cosine()`
- 保留 FTS5，混合召回加权排序

**优势**：
- 同一个 `data/agent.db` 文件，零迁移
- 仍然是嵌入式、单文件、无服务依赖
- 检索性能从 O(n) 降至 O(log n)

**预计工作量**：1-2 天

---

### Phase 2：混合检索 + Rerank

**触发条件**：知识库增长到 10,000+ 条，或评测发现 top-5 召回不稳定

**改动**：
- FTS5 top-30 + Vector top-30 → 合并去重 → 加权排序 → top-5
- 引入 Rerank 模型（BGE reranker / Jina reranker / Cohere）
- 召回评测集（至少覆盖 6 个维度 × 5 query = 30 条 ground truth）
- 独立 embedding 配置（`EMBEDDING_*` 环境变量）

**预计工作量**：3-5 天

---

### Phase 3：alibaba/zvec（评估备选）

**触发条件**：知识库达到 100,000+ 条，或需要多用户隔离 + 高并发读

**评估结论（2026-06-18）**：

| 维度 | 评估 |
|:---|:---|
| 定位 | 阿里开源嵌入式向量数据库（进程内，类 SQLite 架构） |
| Stars | ~11k，Apache 2.0 |
| 核心能力 | DiskANN 索引、原生 FTS、混合检索、WAL 持久化、亿级向量 |
| Node.js SDK | `@zvec/zvec` 声称可用，但 npm 上 403，实际**不可用** |
| 成熟度 | v0.5.x（2024-06），仍处早期 |

**暂不采用原因**：
1. Node.js binding 未发布，无法直接集成到 TS 项目
2. 411 条规模无性能瓶颈，sqlite-vec 已足够
3. SWIG-based binding 增加编译复杂度，与纯 TS 栈冲突
4. 项目偏早期，生态和文档还在完善

**重新评估时机**：
- Node.js SDK 正式发布到 npm
- 知识库增长到需要 DiskANN 的规模（10 万+）
- 需要原生混合检索替代手动 FTS + vector 合并

---

### Phase 4：独立向量服务（远期）

**触发条件**：SaaS 多租户、分布式部署、在线实时更新

**备选方案**：
- **Qdrant**：Rust 实现，gRPC + REST，适合云原生部署
- **Milvus**：分布式架构，适合超大规模（亿级 + 高 QPS）
- **阿里 DashVector**：阿里云托管服务，国内延迟低

此阶段需要解耦知识检索为独立微服务，Agent 通过 gRPC/HTTP 调用。

---

### 路线总结

```
Phase 0 (当前)        Phase 1              Phase 2              Phase 3/4
JS cosine + FTS5  →  sqlite-vec + FTS5  →  + Rerank pipeline  →  zvec / Qdrant
411 条 / <1ms        5k+ 条 / HNSW        10k+ 条 / 混合排序    100k+ / 分布式
────────────────────────────────────────────────────────────────────────────────
纯 TS，零依赖         +1 native ext         +1 rerank API        架构变更
```

每个阶段都是明确的规模触发，不提前优化。
