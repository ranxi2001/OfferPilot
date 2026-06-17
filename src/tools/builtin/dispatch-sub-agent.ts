import type { ToolDefinition, ToolContext } from '../types.js';
import type { SubAgentRuntime } from '../../sub-agent/runtime.js';

export function createDispatchSubAgent(runtime: SubAgentRuntime): ToolDefinition {
  return {
    schema: {
      name: 'dispatch_sub_agent',
      description: '派遣子 Agent 执行专项任务（诊断、追问、研究、报告等），支持并行派遣多个子 Agent',
      parameters: {
        type: 'object',
        properties: {
          agentId: { type: 'string', description: '目标子 Agent ID' },
          input: { type: 'string', description: '派遣给子 Agent 的任务指令' },
          context: { type: 'string', description: '从父会话传递的上下文摘要（可选）' },
        },
        required: ['agentId', 'input'],
      },
    },
    riskLevel: 'low',
    async execute(input: Record<string, unknown>, ctx: ToolContext) {
      const { agentId, input: taskInput, context } = input as {
        agentId: string;
        input: string;
        context?: string;
      };

      const result = await runtime.dispatch({
        agentId,
        input: taskInput,
        parentSessionId: ctx.sessionId,
        context,
      });

      if (!result.success) {
        return {
          success: false,
          isError: true,
          output: JSON.stringify({ error: result.error, agentId }),
        };
      }

      return {
        success: true,
        output: JSON.stringify({
          agentId: result.agentId,
          role: result.role,
          output: result.output,
          tokenUsage: result.tokenUsage,
          duration: result.duration,
        }),
      };
    },
  };
}
