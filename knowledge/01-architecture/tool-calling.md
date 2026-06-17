# 架构设计 - Tool Calling 机制

## Q：Agent 的 Tool Calling 机制是怎么工作的？不同 Provider 的实现有什么差异？

> 来源：某 AI 基础设施团队面试

**新手答**："就是模型调用函数，把参数传过去执行"

**高手答**：

Tool Calling 的本质是让 LLM 输出结构化的调用意图，而非直接执行代码。流程是：

1. **Schema 注册**：开发者用 JSON Schema 描述工具名称、参数类型、描述
2. **模型推理**：LLM 判断需要调用哪个工具，输出结构化的 tool_use block
3. **宿主执行**：应用层解析意图，执行实际函数，将结果作为 tool_result 返回
4. **继续推理**：LLM 拿到结果后决定是继续调用工具还是生成最终回复

Provider 差异：
- **Anthropic**：tool_use 的 input 直接是 object，流式下是 input_json_delta 增量拼接
- **OpenAI**：function_call 的 arguments 是 JSON 字符串，需要自己 parse，容易出 JSON 不完整的坑
- **流式处理**：Anthropic 有 content_block_start/delta/stop 事件边界清晰；OpenAI 的 tool_calls delta 需要按 index 拼接

工程关键点：
- Schema 描述要精准，模糊的 description 会导致模型误调用
- 流式下 JSON 拼接必须处理不完整 JSON 的 edge case
- 并行 tool_call（一次返回多个）要考虑执行策略：独立的可并行，有依赖的要串行
- 超时和重试：工具执行可能阻塞，必须设 timeout

## 考察点

- 对 LLM 调用机制的底层理解
- 跨 Provider 适配经验
- 流式处理的工程细节

## 追问

- 如果模型输出了不合法的 JSON 参数，你怎么处理？
- 你怎么设计一个统一的 Tool 注册和执行层来屏蔽 Provider 差异？
