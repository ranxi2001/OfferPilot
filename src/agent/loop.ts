import type { Message, ParsedResponse, QueryParams, TokenUsage, ToolCall } from '../query-engine/types.js';
import type { QueryEngine } from '../query-engine/engine.js';
import type { ToolRegistry } from '../tools/registry.js';
import type { PermissionGate } from '../permission/gate.js';
import type { ContextManager } from '../context/manager.js';
import type { SessionManager } from '../session/manager.js';
import type { MemoryStore } from '../memory/store.js';
import type { HookPipeline } from '../hooks/pipeline.js';
import { logger } from '../logger.js';

export interface AgentConfig {
  queryEngine: QueryEngine;
  toolRegistry: ToolRegistry;
  permissionGate: PermissionGate;
  contextManager: ContextManager;
  sessionManager: SessionManager;
  memoryStore: MemoryStore;
  hookPipeline?: HookPipeline;
  maxIterations?: number;
  maxBudgetTokens?: number;
  defaultModel?: string;
  abortSignal?: AbortSignal;
  onTextDelta?: (text: string) => void;
  onToolCall?: (name: string, input: Record<string, unknown>) => void;
  onToolResult?: (name: string, result: string) => void;
  onPermissionRequest?: (toolName: string, input: Record<string, unknown>) => Promise<boolean>;
}

export interface UsageStats {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  iterations: number;
}

export class AgentLoop {
  private config: AgentConfig;
  private maxIterations: number;
  private usage: UsageStats = { inputTokens: 0, outputTokens: 0, totalTokens: 0, iterations: 0 };

  constructor(config: AgentConfig) {
    this.config = config;
    this.maxIterations = config.maxIterations ?? 10;
  }

  getUsage(): UsageStats {
    return { ...this.usage };
  }

  async run(sessionId: string, userMessage: string): Promise<string> {
    const { queryEngine, toolRegistry, contextManager, sessionManager, memoryStore } = this.config;
    const abortSignal = this.config.abortSignal;

    const log = logger.child({ sessionId, component: 'AgentLoop' });

    const session = sessionManager.get(sessionId);
    if (!session) throw new Error(`Session ${sessionId} not found`);

    if (session.state === 'idle') {
      sessionManager.transition(sessionId, 'active');
    }

    log.info('run started', { messageLength: userMessage.length });
    sessionManager.addMessage(sessionId, { role: 'user', content: userMessage });

    const memories = memoryStore.query({ sessionId, limit: 5 });
    if (memories.length > 0) {
      const memoryText = memories.map((m) => `- [${m.type}] ${m.content}`).join('\n');
      contextManager.setLayer('memory', `User context:\n${memoryText}`);
    }

    const systemPrompt = contextManager.buildSystemPrompt();
    let messages = [...session.messages];
    let finalText = '';

    for (let i = 0; i < this.maxIterations; i++) {
      if (abortSignal?.aborted) {
        throw new Error('Agent loop aborted');
      }

      if (this.config.maxBudgetTokens && this.usage.totalTokens >= this.config.maxBudgetTokens) {
        break;
      }

      const compressed = contextManager.compress(messages);
      if (compressed.level !== 'none') {
        messages = compressed.messages;
        sessionManager.replaceMessages(sessionId, messages);
      }

      const params: QueryParams = {
        model: this.config.defaultModel,
        messages,
        tools: toolRegistry.listSchemas(),
        systemPrompt,
        onTextDelta: this.config.onTextDelta,
      };

      const response: ParsedResponse = await queryEngine.query(params);
      this.trackUsage(response.usage);
      this.usage.iterations = i + 1;

      if (response.type === 'text') {
        finalText = response.content ?? '';
        sessionManager.addMessage(sessionId, { role: 'assistant', content: finalText });
        break;
      }

      if (response.type === 'tool_use' && response.toolCalls) {
        const assistantMsg: Message = {
          role: 'assistant',
          content: response.content,
          toolCalls: response.toolCalls,
        };
        sessionManager.addMessage(sessionId, assistantMsg);
        messages.push(assistantMsg);

        const toolResults = await Promise.all(
          response.toolCalls.map((toolCall) => this.executeTool(toolCall, sessionId))
        );

        for (let j = 0; j < response.toolCalls.length; j++) {
          const toolMsg: Message = {
            role: 'tool',
            toolCallId: response.toolCalls[j].id,
            content: toolResults[j].output,
            isError: toolResults[j].isError,
          };
          sessionManager.addMessage(sessionId, toolMsg);
          messages.push(toolMsg);
        }
      }
    }

    if (!finalText) {
      finalText = '抱歉，我在处理您的请求时达到了最大迭代次数，未能生成完整回复。请尝试简化问题或重新提问。';
      sessionManager.addMessage(sessionId, { role: 'assistant', content: finalText });
      log.warn('max iterations reached', { iterations: this.usage.iterations });
    }

    log.info('run completed', { iterations: this.usage.iterations, totalTokens: this.usage.totalTokens });
    return finalText;
  }

  private trackUsage(usage: TokenUsage): void {
    this.usage.inputTokens += usage.inputTokens;
    this.usage.outputTokens += usage.outputTokens;
    this.usage.totalTokens += usage.inputTokens + usage.outputTokens;
  }

  private async executeTool(
    toolCall: ToolCall,
    sessionId: string
  ): Promise<{ output: string; isError?: boolean }> {
    const { toolRegistry, permissionGate, hookPipeline } = this.config;

    const tool = toolRegistry.get(toolCall.name);
    if (!tool) {
      return { output: `Tool "${toolCall.name}" not found`, isError: true };
    }

    const decision = permissionGate.check(toolCall.name, tool.riskLevel, sessionId);
    if (!decision.allowed) {
      return { output: `Permission denied: ${decision.reason}`, isError: true };
    }

    if (decision.requiresUserConfirm) {
      if (!this.config.onPermissionRequest) {
        return { output: `Tool "${toolCall.name}" requires user confirmation but no handler is configured`, isError: true };
      }
      const confirmed = await this.config.onPermissionRequest(toolCall.name, toolCall.input);
      if (!confirmed) {
        return { output: `User denied execution of "${toolCall.name}"`, isError: true };
      }
    }

    let input = toolCall.input;

    if (hookPipeline) {
      const preResult = await hookPipeline.runPreTool({
        sessionId,
        toolName: toolCall.name,
        input,
      });
      if (!preResult.proceed) {
        return { output: `Skipped by hook: ${preResult.reason}`, isError: true };
      }
      input = preResult.input;
    }

    this.config.onToolCall?.(toolCall.name, input);

    const toolStart = Date.now();
    let result = await toolRegistry.execute(toolCall.name, input, { sessionId });

    if (hookPipeline) {
      result = await hookPipeline.runPostTool({
        sessionId,
        toolName: toolCall.name,
        input,
        result,
      });
    }

    permissionGate.recordAudit({
      timestamp: Date.now(),
      sessionId,
      toolName: toolCall.name,
      riskLevel: tool.riskLevel,
      decision: decision.requiresUserConfirm ? 'confirmed' : 'allowed',
      input,
    });

    this.config.onToolResult?.(toolCall.name, result.output);

    logger.debug('tool executed', { tool: toolCall.name, duration: Date.now() - toolStart, success: result.success });

    return { output: result.output, isError: result.isError ?? !result.success };
  }
}
