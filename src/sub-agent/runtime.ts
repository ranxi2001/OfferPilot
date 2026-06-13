import type { QueryEngine } from '../query-engine/engine.js';
import type { Message } from '../query-engine/types.js';
import { ConcurrencyPool } from './pool.js';
import type { SubAgentConfig, SubAgentResult, SubAgentTask } from './types.js';

const ROLE_PROMPTS: Record<string, string> = {
  diagnostician: `你是诊断专家子 Agent。你的任务是对面试回答进行深度分析，找出知识盲点、逻辑断层和表达不足。输出结构化诊断结果。`,
  interviewer: `你是模拟面试官子 Agent。根据候选人的回答，生成有针对性的追问。追问应该逐步深入，检验真实理解深度。`,
  researcher: `你是知识研究子 Agent。负责从知识库中查找相关参考资料，整合多个来源的信息，为诊断提供依据。`,
  reporter: `你是报告生成子 Agent。负责将诊断结果、评分和建议整理成结构化的会话报告。`,
  'jd-analyst': `你是 JD 分析子 Agent。解析职位描述，提取核心技术要求、团队背景、职级信号和面试准备方向。输出结构化分析。`,
  'resume-optimizer': `你是简历优化子 Agent。基于目标岗位，对简历各段落提出改进建议：量化表达、STAR 结构、关键词补充、信息密度优化。`,
  'gap-analyzer': `你是差距分析子 Agent。对比简历和 JD，找出技能匹配项和差距项，生成针对性的补强方案和面试准备策略。`,
};

export class SubAgentRuntime {
  private pool: ConcurrencyPool;
  private agents = new Map<string, SubAgentConfig>();
  private queryEngine: QueryEngine;

  constructor(queryEngine: QueryEngine, maxConcurrency = 3) {
    this.queryEngine = queryEngine;
    this.pool = new ConcurrencyPool(maxConcurrency);
  }

  register(config: SubAgentConfig): void {
    this.agents.set(config.id, {
      ...config,
      systemPrompt: config.systemPrompt || ROLE_PROMPTS[config.role] || '',
    });
  }

  async dispatch(task: SubAgentTask): Promise<SubAgentResult> {
    const config = this.agents.get(task.agentId);
    if (!config) {
      return {
        agentId: task.agentId,
        role: 'researcher',
        output: '',
        tokenUsage: { input: 0, output: 0 },
        duration: 0,
        success: false,
        error: `Agent "${task.agentId}" not registered`,
      };
    }

    return this.pool.run(() => this.execute(config, task));
  }

  async dispatchAll(tasks: SubAgentTask[]): Promise<SubAgentResult[]> {
    return Promise.all(tasks.map((t) => this.dispatch(t)));
  }

  private async execute(config: SubAgentConfig, task: SubAgentTask): Promise<SubAgentResult> {
    const start = Date.now();

    const messages: Message[] = [{ role: 'user', content: task.input }];

    try {
      const response = await this.queryEngine.query({
        model: config.model,
        messages,
        systemPrompt: config.systemPrompt,
        maxTokens: 2048,
      });

      return {
        agentId: config.id,
        role: config.role,
        output: response.content ?? '',
        tokenUsage: {
          input: response.usage.inputTokens,
          output: response.usage.outputTokens,
        },
        duration: Date.now() - start,
        success: true,
      };
    } catch (err) {
      return {
        agentId: config.id,
        role: config.role,
        output: '',
        tokenUsage: { input: 0, output: 0 },
        duration: Date.now() - start,
        success: false,
        error: (err as Error).message,
      };
    }
  }
}
