import type { ToolDefinition } from '../types.js';
import { RealtimeInterviewSession } from '../../realtime/index.js';

const sessions = new Map<string, RealtimeInterviewSession>();

export const realtimeInterview: ToolDefinition = {
  schema: {
    name: 'realtime_interview',
    description: '实时面试模拟：提问 → 收答 → 即时缺陷分析 → TTS 播报。支持 start/answer/report 三种 action',
    parameters: {
      type: 'object',
      properties: {
        action: {
          type: 'string',
          enum: ['start', 'answer', 'next', 'report'],
          description: 'start=开始新模拟, answer=提交回答, next=下一题, report=总结报告',
        },
        sessionId: { type: 'string', description: '会话 ID（answer/next/report 时必填）' },
        questions: {
          type: 'array',
          description: '自定义题目列表（start 时可选）',
        },
        answerText: { type: 'string', description: '候选人回答文本（answer 时必填）' },
      },
      required: ['action'],
    },
  },
  riskLevel: 'low',
  async execute(input) {
    const { action, sessionId, questions, answerText } = input as {
      action: string;
      sessionId?: string;
      questions?: string[];
      answerText?: string;
    };

    switch (action) {
      case 'start': {
        const session = new RealtimeInterviewSession(questions);
        sessions.set(session.id, session);
        const firstQuestion = session.start();
        return {
          success: true,
          output: JSON.stringify({
            sessionId: session.id,
            state: 'questioning',
            currentQuestion: firstQuestion,
            progress: session.progress,
            ttsText: `面试开始。第一题：${firstQuestion}`,
          }),
        };
      }

      case 'answer': {
        const session = sessions.get(sessionId ?? '');
        if (!session) return { success: false, output: '会话不存在，请先 start' };
        if (!answerText) return { success: false, output: '请提供回答文本 answerText' };

        const { defects, summary } = session.submitAnswer(answerText);
        return {
          success: true,
          output: JSON.stringify({
            sessionId: session.id,
            state: 'analyzed',
            defectsCount: defects.length,
            defects: defects.map((d) => ({
              type: d.type,
              severity: d.severity,
              description: d.description,
              suggestion: d.suggestion,
            })),
            summary,
            progress: session.progress,
            ttsText: summary,
          }),
        };
      }

      case 'next': {
        const session = sessions.get(sessionId ?? '');
        if (!session) return { success: false, output: '会话不存在' };

        const nextQ = session.nextQuestion();
        if (!nextQ) {
          return {
            success: true,
            output: JSON.stringify({
              sessionId: session.id,
              state: 'finished',
              message: '所有题目已问完，请使用 report 查看总结',
              ttsText: '面试模拟结束，输入 report 查看综合评价',
            }),
          };
        }
        return {
          success: true,
          output: JSON.stringify({
            sessionId: session.id,
            state: 'questioning',
            currentQuestion: nextQ,
            progress: session.progress,
            ttsText: `下一题：${nextQ}`,
          }),
        };
      }

      case 'report': {
        const session = sessions.get(sessionId ?? '');
        if (!session) return { success: false, output: '会话不存在' };

        const report = session.getReport();
        sessions.delete(sessionId!);
        return {
          success: true,
          output: JSON.stringify({
            ...report,
            ttsText: `面试模拟结束。综合评分 ${report.overallScore}/10，共发现 ${report.totalDefects} 个缺陷。主要问题：${report.topIssues.join('、')}`,
          }),
        };
      }

      default:
        return { success: false, output: `未知 action: ${action}` };
    }
  },
};
