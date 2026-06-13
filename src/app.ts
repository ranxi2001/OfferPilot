import { QueryEngine } from './query-engine/index.js';
import { ClaudeProvider } from './query-engine/providers/claude.js';
import { OpenAIProvider } from './query-engine/providers/openai.js';
import { DeepSeekProvider } from './query-engine/providers/deepseek.js';
import { MockProvider } from './query-engine/providers/mock.js';
import { createToolRegistry } from './tools/index.js';
import { PermissionGate } from './permission/index.js';
import { ContextManager } from './context/index.js';
import { SessionManager } from './session/index.js';
import { MemoryStore } from './memory/index.js';
import { AgentLoop } from './agent/index.js';
import { HookPipeline, inputSanitizerHook, tokenCounterHook } from './hooks/index.js';
import {
  CommandParser,
  helpCommand,
  statusCommand,
  dimensionsCommand,
  quitCommand,
  resetCommand,
} from './command/index.js';

const SYSTEM_PROMPT = `你是 OfferPilot，一个全链路求职辅导 Agent，专注于 AI Agent / LLM 工程方向。

你的核心能力：

【面试诊断】
1. 搜索知识库中的 385+ 道真实面试题及高手答案
2. 对用户的回答进行结构化诊断（评分 + 差距分析 + 改进建议）
3. 模拟面试官追问，检验理解深度
4. 对比用户答案与专家答案的差距

【JD 分析】
5. 解析职位描述，提取技术栈要求、职级信号、团队信息
6. 生成针对该 JD 的面试准备重点

【简历优化】
7. 分析简历与 JD 的匹配度，找出差距项
8. 对简历段落提出优化建议（量化、STAR、关键词）
9. 根据目标 JD 定向包装简历亮点

【模拟面试】
10. 根据 JD + 简历生成个性化面试题序列
11. 推荐个性化学习路径

【实时面试模拟】
12. 面试官提问 → TTS 语音播报
13. 候选人作答 → 实时缺陷检测（结构/深度/案例/口头禅/偏题/模糊等）
14. 每题即时反馈 + 改进建议
15. 全场总结报告（评分 + 缺陷分布 + 高频问题）

工作方式：
- 用户贴入 JD → 自动解析并给出面试准备方向
- 用户贴入简历 + JD → 匹配度分析 + 简历优化建议
- 用户输入面试题 + 回答 → 诊断评分 + 对比 + 追问
- 用户说"模拟面试" → 生成题目序列并逐题诊断
- 用户说"面试模拟"/"实时面试" → 启动实时面试模拟，逐题 TTS 提问 + 实时缺陷分析`;

export interface AppOptions {
  model?: string;
  onTextDelta?: (text: string) => void;
  onToolCall?: (name: string, input: Record<string, unknown>) => void;
  onToolResult?: (name: string, result: string) => void;
}

export function createApp(opts?: AppOptions) {
  const providers = buildProviders();

  const queryEngine = new QueryEngine({
    providers,
    retry: { maxRetries: 3 },
  });

  const toolRegistry = createToolRegistry();
  const permissionGate = new PermissionGate();
  const contextManager = new ContextManager();
  const sessionManager = new SessionManager();
  const memoryStore = new MemoryStore();

  const hookPipeline = new HookPipeline();
  hookPipeline.register(inputSanitizerHook);
  hookPipeline.register(tokenCounterHook);

  const commandParser = new CommandParser();
  commandParser.register(helpCommand);
  commandParser.register(statusCommand);
  commandParser.register(dimensionsCommand);
  commandParser.register(quitCommand);
  commandParser.register(resetCommand);

  contextManager.setLayer('system', SYSTEM_PROMPT);

  const agent = new AgentLoop({
    queryEngine,
    toolRegistry,
    permissionGate,
    contextManager,
    sessionManager,
    memoryStore,
    hookPipeline,
    defaultModel: opts?.model,
    onTextDelta: opts?.onTextDelta,
    onToolCall: opts?.onToolCall,
    onToolResult: opts?.onToolResult,
  });

  return { agent, sessionManager, queryEngine, toolRegistry, memoryStore, commandParser, hookPipeline };
}

function buildProviders() {
  const configs = [];

  if (process.env.ANTHROPIC_API_KEY) {
    configs.push({
      provider: new ClaudeProvider(),
      models: ['claude-sonnet-4-20250514', 'claude-haiku-4-5-20251001', 'claude-opus-4-20250514'],
      defaultModel: 'claude-sonnet-4-20250514',
    });
  }

  if (process.env.OPENAI_API_KEY) {
    configs.push({
      provider: new OpenAIProvider(),
      models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo'],
      defaultModel: 'gpt-4o',
    });
  }

  if (process.env.DEEPSEEK_API_KEY) {
    configs.push({
      provider: new DeepSeekProvider(),
      models: ['deepseek-chat', 'deepseek-coder'],
      defaultModel: 'deepseek-chat',
    });
  }

  // Fallback to mock provider when no API keys are configured
  if (configs.length === 0) {
    configs.push({
      provider: new MockProvider(),
      models: ['mock'],
      defaultModel: 'mock',
    });
  }

  return configs;
}
