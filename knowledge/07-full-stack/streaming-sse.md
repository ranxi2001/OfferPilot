# 全栈工程 - SSE 流式传输

## Q：如何实现 LLM 的流式输出到前端？SSE vs WebSocket 怎么选？

> 来源：某全栈团队面试

**新手答**："用 WebSocket 推送"

**高手答**：

对于 LLM 的单向流式输出，SSE (Server-Sent Events) 是更合适的选择：

**SSE vs WebSocket**：

| 维度 | SSE | WebSocket |
|------|-----|-----------|
| 方向 | 单向（服务器→客户端） | 双向 |
| 协议 | HTTP/1.1+ | 独立协议 |
| 重连 | 浏览器自动重连 | 需手动实现 |
| 代理兼容 | 好（普通 HTTP） | 差（需升级协议） |
| Serverless | 支持（streaming response） | 不支持 |

LLM 场景是典型的单向流：用户发一个请求，服务端逐 token 推送。SSE 刚好匹配。

**工程实现要点**：

1. **服务端**：
   - Content-Type: text/event-stream
   - 每个事件格式：`data: {json}\n\n`
   - 心跳：每 15s 发 `: ping\n\n`（冒号开头是 SSE 注释，不触发事件）
   - 结束标记：`data: [DONE]\n\n`

2. **客户端**：
   - 用 `EventSource` API 或 `fetch` + `ReadableStream`
   - EventSource 有自动重连但不支持 POST 和自定义 header
   - 生产中用 fetch streaming：可以带 auth header，支持 POST body

3. **Serverless 坑**：
   - Vercel/Cloudflare 有响应时间限制（通常 30s-60s）
   - 解决：heartbeat 保持连接 + 客户端超时重连
   - 或者用 Edge Runtime（执行时间更长）

4. **错误处理**：
   - 中间断开：客户端检测 ReadableStream 关闭，展示已收到的内容
   - 服务端错误：发 error 事件后关闭流
   - Token 计数：在 [DONE] 事件中附带 usage 信息

## 考察点

- 对实时通信方案的理解
- Serverless 环境的工程经验
- 端到端的流式架构设计

## 追问

- 如果用户刷新页面，正在生成的回答怎么恢复？
- 你怎么测试 SSE 的边界情况（网络中断、超长响应）？
