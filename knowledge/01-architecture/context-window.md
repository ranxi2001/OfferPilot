# 架构设计 - Context Window 管理

## Q：Agent 对话越来越长时，如何管理 Context Window？

> 来源：某 AI 产品团队面试

**新手答**："超过限制就截断前面的消息"

**高手答**：

Context Window 管理是 Agent 工程中最容易被忽视但影响最大的问题。核心挑战是在有限窗口内保持对话连贯性。

分层策略：

1. **Token 计数**：不能靠字符数估算，要用 tiktoken 或 Provider 的 countTokens API 精确计算。不同模型的 tokenizer 不同。

2. **Sliding Window**：保留最近 N 轮完整对话，丢弃更早的。简单但会丢失关键上下文（比如用户第一轮说的偏好）。

3. **Summarization**：对旧消息调用 LLM 生成摘要，压缩到 1/10 体积。关键是保留"什么信息必须留"——用户偏好、关键决策、当前任务目标。

4. **分层上下文**：
   - System prompt（固定，高优先级）
   - Long-term memory（从 DB 查询，按相关性）
   - Session history（最近对话）
   - Immediate context（当前轮的输入/工具结果）

5. **预算分配**：给每层分配 token 预算，动态调整。比如 system 20%，memory 15%，history 50%，immediate 15%。

工程踩坑：
- Tool result 可能很长（比如搜索返回大段文本），必须截断或摘要
- 压缩时不能破坏 tool_use → tool_result 的配对关系
- 多轮对话中 token 增长是超线性的（每轮都重复发所有历史）

## 考察点

- 对 token 经济学的理解
- 分层设计能力
- 实际工程中的 trade-off 决策

## 追问

- 你的压缩策略会不会导致模型"遗忘"重要信息？怎么缓解？
- 如果用户提到"刚才说的那个方案"，而那个方案已经被压缩了，怎么办？
